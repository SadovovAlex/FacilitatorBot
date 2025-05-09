//return -1001225930156, nil

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	godotenv "github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

const CHECK_HOURS = -6         // hours get DB messages
const AI_REQUEST_TIMEOUT = 300 // seconds for AI request

// Config структура для конфигурации бота
type Config struct {
	TelegramToken string
	LocalLLMUrl   string // URL локальной LLM (например "http://localhost:1234/v1/chat/completions")
	AllowedGroups []int64
	SummaryPrompt string
	SystemPrompt  string
	AnekdotPrompt string
	HistoryDays   int    // Сколько дней хранить историю
	DBPath        string // Путь к файлу SQLite
}

// Bot структура основного бота
type Bot struct {
	config        Config
	tgBot         *tgbotapi.BotAPI
	httpClient    *http.Client
	db            *sql.DB
	chatHistories map[int64][]ChatMessage // История сообщений по чатам
	lastSummary   map[int64]time.Time     // Время последней сводки по чатам
}

// ChatMessage структура для хранения сообщений
type ChatMessage struct {
	User string
	Text string
	Time time.Time
}

// DB структуры
type DBChat struct {
	ID       int64
	Title    string
	Type     string
	Username string
}

type DBUser struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
}

// DBMessage структура для хранения сообщений из БД
type DBMessage struct {
	ID        int
	ChatID    int64
	UserID    int64
	Text      string
	Timestamp int64
	Username  string
	ChatTitle string
}

// LocalLLMRequest структура запроса к локальной LLM
type LocalLLMRequest struct {
	Model       string            `json:"model"`
	Messages    []LocalLLMMessage `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

// LocalLLMMessage структура сообщения для LLM
type LocalLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LocalLLMResponse структура ответа от LLM
type LocalLLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func main() {
	// Загрузка переменных окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		log.Printf("Ошибка загрузки .env файла: %v (продолжаем с переменными окружения)", err)
	}

	// Загрузка конфигурации
	config := Config{
		TelegramToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		LocalLLMUrl:   getEnv("LOCAL_LLM_URL", "http://localhost:1234/v1/chat/completions"),
		AllowedGroups: []int64{},
		//SummaryPrompt: "Создай краткую сводку обсуждения. Выдели ключевые темы обсуждения. Авторы сообщений в формате @username. Будь  информативным. Используй только эти сообщения:\n%s",
		//SystemPrompt:  "Ты полезный ассистент, который создает краткие содержательные пересказы обсуждений в чатах. Выделяющий тему и суть разговора.",
		//AnekdotPrompt: "Используя предоставленные сообщения пользователей, придумайте короткий, забавный анекдот, частично связанный с обсуждением. Напиши анекдот в виде одного законченного текста. Не используй в тексте анекдота username, придумай:\n%s",
		HistoryDays:   1,
		DBPath:        getEnv("DB_PATH", "telegram_bot.db"),
		SummaryPrompt: "Generate concise Russian summary of discussion. Highlight key topics. Format authors as @username. Use only these messages:\n%s\nReply in Russian.",
		SystemPrompt:  "You're an AI assistant that creates concise Russian summaries of chat discussions. Identify main topics and essence. Always reply in Russian.",
		AnekdotPrompt: "Using these messages, create a short funny joke in Russian, loosely related to discussion. Format as one cohesive text. Don't use usernames:\n%s\nReply in Russian only.",
	}

	// Проверка обязательных переменных
	if config.TelegramToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен")
	}

	fmt.Printf("config.TelegramToken: %v\n", config.TelegramToken)

	// Инициализация бота
	bot, err := NewBot(config)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}

	// Инициализация БД
	err = bot.initDB()
	if err != nil {
		log.Fatalf("Ошибка инициализации базы данных: %v", err)
	}
	defer bot.db.Close()

	// Запуск бота
	bot.Run()
}

// getEnv возвращает значение переменной окружения или значение по умолчанию
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// NewBot создает новый экземпляр бота
func NewBot(config Config) (*Bot, error) {
	tgBot, err := tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания Telegram бота: %v", err)
	}

	db, err := sql.Open("sqlite3", config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия базы данных: %v", err)
	}

	return &Bot{
		config:        config,
		tgBot:         tgBot,
		httpClient:    &http.Client{Timeout: AI_REQUEST_TIMEOUT * time.Second},
		db:            db,
		chatHistories: make(map[int64][]ChatMessage),
		lastSummary:   make(map[int64]time.Time),
	}, nil
}

// Run запускает бота
func (b *Bot) Run() {
	log.Printf("Бот запущен как %s", b.tgBot.Self.UserName)

	// Основной цикл обработки обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.tgBot.GetUpdatesChan(u)

	// Очистка старых сообщений
	go b.cleanupOldMessages()

	for update := range updates {
		if update.Message != nil {
			// Форматированный вывод ненулевых полей сообщения
			fmt.Println("=== Новое сообщение ===")
			if update.Message.MessageID != 0 {
				fmt.Printf("ID сообщения: %d\n", update.Message.MessageID)
			}
			if update.Message.From != nil {
				fmt.Printf("От: %s (ID: %d)\n",
					getUserName(update.Message.From),
					update.Message.From.ID)
			}
			if update.Message.Chat != nil {
				fmt.Printf("Чат: %s (ID: %d, тип: %s)\n",
					getChatTitle(update.Message.Chat),
					update.Message.Chat.ID,
					update.Message.Chat.Type)
			}
			if update.Message.Text != "" {
				fmt.Printf("Текст: %s\n", update.Message.Text)
			}
			if update.Message.Caption != "" {
				fmt.Printf("Подпись: %s\n", update.Message.Caption)
			}

			if update.Message.Date != 0 {
				// Конвертируем Unix timestamp в time.Time
				msgTime := time.Unix(int64(update.Message.Date), 0)
				fmt.Printf("Дата: %s\n", msgTime.Format("2006-01-02 15:04:05"))
			}

			if update.Message.ReplyToMessage != nil {
				fmt.Printf("Ответ на сообщение ID: %d\n", update.Message.ReplyToMessage.MessageID)
			}
			if update.Message.ForwardFromChat != nil {
				fmt.Printf("Переслано из чата: %s (ID: %d)\n",
					update.Message.ForwardFromChat.Title,
					update.Message.ForwardFromChat.ID)
			}
			fmt.Println("======================")

			// Обработка сообщения
			b.processMessage(update.Message)
		}
	}
}

// initDB инициализирует базу данных
func (b *Bot) initDB() error {
	// Создаем таблицу чатов
	_, err := b.db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			id INTEGER PRIMARY KEY,
			title TEXT,
			type TEXT,
			username TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы чатов: %v", err)
	}

	// Создаем таблицу пользователей
	_, err = b.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы пользователей: %v", err)
	}

	// Создаем таблицу сообщений
	_, err = b.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY,
			chat_id INTEGER,
			user_id INTEGER,
			text TEXT,
			timestamp INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(chat_id) REFERENCES chats(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы сообщений: %v", err)
	}

	return nil
}

// saveChat сохраняет информацию о чате в БД
func (b *Bot) saveChat(chat *tgbotapi.Chat) error {
	if chat == nil {
		return nil
	}

	_, err := b.db.Exec(`
		INSERT OR IGNORE INTO chats (id, title, type, username) 
		VALUES (?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Type, chat.UserName)

	return err
}

// saveUser сохраняет информацию о пользователе в БД
func (b *Bot) saveUser(user *tgbotapi.User) error {
	if user == nil {
		return nil
	}

	_, err := b.db.Exec(`
		INSERT OR IGNORE INTO users (id, username, first_name, last_name) 
		VALUES (?, ?, ?, ?)`,
		user.ID, user.UserName, user.FirstName, user.LastName)

	return err
}

// saveMessage сохраняет сообщение в БД
func (b *Bot) saveMessage(chatID, userID int64, text string, timestamp int64) error {
	_, err := b.db.Exec(`
		INSERT INTO messages (chat_id, user_id, text, timestamp) 
		VALUES (?, ?, ?, ?)`,
		chatID, userID, text, timestamp)

	return err
}

// getRecentMessages получает сообщения за последние 6 часов
func (b *Bot) getRecentMessages(chatID int64, limit int) ([]DBMessage, error) {
	sixHoursAgo := time.Now().Add(CHECK_HOURS * time.Hour).Unix()

	// Если лимит не задан, устанавливаем его в 0, чтобы получить все сообщения
	if limit == 0 {
		limit = -1
	}

	query := `
		SELECT m.id, m.chat_id, m.user_id, m.text, m.timestamp, 
		       u.username, c.title as chat_title
		FROM messages m
		LEFT JOIN users u ON m.user_id = u.id
		LEFT JOIN chats c ON m.chat_id = c.id
		WHERE m.timestamp >= ? AND m.chat_id = ?
		ORDER BY m.timestamp desc
		LIMIT ?
	`

	rows, err := b.db.Query(query, sixHoursAgo, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса сообщений: %v", err)
	}
	defer rows.Close()

	var messages []DBMessage
	for rows.Next() {
		var msg DBMessage
		err := rows.Scan(
			&msg.ID,
			&msg.ChatID,
			&msg.UserID,
			&msg.Text,
			&msg.Timestamp,
			&msg.Username,
			&msg.ChatTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения сообщения: %v", err)
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка обработки результатов: %v", err)
	}

	return messages, nil
}

// =======  tg Вспомогательная функция для получения имени пользователя
func getUserName(user *tgbotapi.User) string {
	if user == nil {
		return "Unknown"
	}
	if user.UserName != "" {
		return "@" + user.UserName
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", user.FirstName, user.LastName))
}

// Вспомогательная функция для получения названия чата
func getChatTitle(chat *tgbotapi.Chat) string {
	if chat == nil {
		return "Unknown"
	}
	if chat.Title != "" {
		return chat.Title
	}
	return getUserName(&tgbotapi.User{
		FirstName: chat.FirstName,
		LastName:  chat.LastName,
		UserName:  chat.UserName,
	})
}

// processMessage обрабатывает входящие сообщения
func (b *Bot) processMessage(message *tgbotapi.Message) {
	// fmt.Printf("FromChat:%v\n", message.ForwardFromChat)
	// fmt.Printf("Chat:%v\n", message.Chat)
	// fmt.Printf("сообщение: %++v\n", message)

	// Обработка команд
	if message.IsCommand() {
		b.handleCommand(message)
		return
	}

	// Сохранение сообщений из групп
	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		b.storeMessage(message)
	}
}

func (b *Bot) canBotReadMessages(chatID int64) bool {
	member, err := b.tgBot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: b.tgBot.Self.ID,
		},
	})

	if err != nil {
		log.Printf("Ошибка проверки прав: %v", err)
		return false
	}

	// Бот может читать сообщения если он администратор или обычный участник
	return member.Status == "administrator" || member.Status == "member"
}

// handleCommand обрабатывает команды бота
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	// Разрешенные пользователи
	allowedUsers := map[int64]bool{
		152657363: true, //@wrwfx
		233088195: true,
	}

	// Проверяем, есть ли пользователь в списке разрешенных
	if message.From != nil && !allowedUsers[message.From.ID] {
		b.sendMessage(message.Chat.ID, "У вас нет прав для использования этого бота.")
		return
	}

	// Проверяем, может ли бот видеть сообщения в этом чате
	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		if !b.canBotReadMessages(message.Chat.ID) {
			b.sendMessage(message.Chat.ID, "Мне нужны права администратора или участника в этой группе чтобы видеть сообщения.")
			return
		}
	}

	switch message.Command() {
	case "start":
		b.sendMessage(message.Chat.ID, "Привет! Я бот для создания кратких пересказов обсуждений. Используй /summary для получения сводки.")
	case "help":
		b.sendMessage(message.Chat.ID, "Доступные команды:\n/summary - получить сводку обсуждений\n/summary_from - получить сводку из другого чата (ответьте на это сообщение, переслав сообщение из нужного чата)\n/stats - статистика по сохраненным сообщениям\n/anekdot - придумаю анекдот по темам обсуждения =)")
	case "summary":
		b.handleSummaryRequest(message)
	case "summary_from":
		b.handleSummaryFromRequest(message)
	case "stats":
		b.handleStatsRequest(message)
	case "anekdot":
		b.handleAnekdotRequest(message)
	default:
		b.sendMessage(message.Chat.ID, "Неизвестная команда. Используйте /help для списка команд.")
	}
}

// handleSummaryRequest обрабатывает запрос на сводку текущего чата
func (b *Bot) handleSummaryRequest(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	//messages, err := b.getRecentMessages(chatID)
	messages, err := b.getRecentMessages(-1002478281670, 100) //Выборка из БД только Атипичный Чат
	if err != nil {
		fmt.Printf("ошибка получения сообщений: %v", err)
		return
	}

	if len(messages) == 0 {
		message := fmt.Sprintf("Нет сообщений за последние %v часов, я похоже спал =)", CHECK_HOURS*-1)
		fmt.Println(message)
		b.sendMessage(chatID, message)
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		msgTime := time.Unix(msg.Timestamp, 0)
		fmt.Fprintf(&messagesText, "[%s] %s: %s\n",
			msgTime.Format("15:04"),
			msg.Username,
			msg.Text)
	}

	fmt.Println(messagesText.String())

	// Создание сводки с помощью локальной LLM
	summary, err := b.generateSummary(messagesText.String())
	if err != nil {
		log.Printf("Ошибка генерации сводки: %v", err)
		b.sendMessage(chatID, "Произошла ошибка при создании сводки.")
		return
	}

	fmt.Printf("Resp AI: %v", summary)

	b.sendMessage(chatID, "📝 Сводка обсуждений:\n\n"+summary)
	b.lastSummary[chatID] = time.Now()
}

// handleSummaryFromRequest обрабатывает запрос на сводку из другого чата
func (b *Bot) handleSummaryFromRequest(message *tgbotapi.Message) {
	if message.ReplyToMessage == nil || message.ReplyToMessage.ForwardFromChat == nil {
		b.sendMessage(message.Chat.ID, "Пожалуйста, ответьте на это сообщение, переслав сообщение из чата, для которого нужно сделать сводку.")
		return
	}

	sourceChatID := message.ReplyToMessage.ForwardFromChat.ID
	history := b.chatHistories[sourceChatID]

	if len(history) == 0 {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Нет данных для чата %s.", message.ReplyToMessage.ForwardFromChat.Title))
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range history {
		fmt.Fprintf(&messagesText, "[%s] %s: %s\n",
			msg.Time.Format("15:04"), msg.User, msg.Text)
	}

	summary, err := b.generateSummary(messagesText.String())
	if err != nil {
		log.Printf("Ошибка генерации сводки: %v", err)
		b.sendMessage(message.Chat.ID, "Произошла ошибка при создании сводки.")
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("📝 Краткая сводка из %s:\n\n%s",
		message.ReplyToMessage.ForwardFromChat.Title, summary))
}

// handleSummaryRequest обрабатывает запрос на сводку текущего чата
func (b *Bot) handleAnekdotRequest(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	//messages, err := b.getRecentMessages(chatID)
	messages, err := b.getRecentMessages(-1002478281670, 10) //Выборка из БД только Атипичный Чат
	if err != nil {
		fmt.Printf("ошибка получения сообщений: %v", err)
		return
	}

	if len(messages) == 0 {
		fmt.Printf("Нет сообщений за последние 6 часов")
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		//msgTime := time.Unix(msg.Timestamp, 0)
		fmt.Fprintf(&messagesText, "%s: %s\n",
			//msgTime.Format("15:04"),
			msg.Username,
			msg.Text)
	}

	fmt.Println(messagesText.String())

	// Создание сводки с помощью локальной LLM
	summary, err := b.generateAnekdot(messagesText.String())
	if err != nil {
		log.Printf("Ошибка генерации анекдота: %v", err)
		b.sendMessage(chatID, "Не смог придумать анекдот, попробуй позже.")
		return
	}

	fmt.Printf("Resp AI: %v", summary)

	b.sendMessage(chatID, "📝 Аnekdot:\n\n"+summary)
	b.lastSummary[chatID] = time.Now()
}

// handleStatsRequest показывает статистику по сообщениям из БД
func (b *Bot) handleStatsRequest(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// 1. Получаем общее количество сообщений в чате
	var totalMessages int
	err := b.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&totalMessages)
	if err != nil {
		log.Printf("Ошибка получения общего количества сообщений: %v", err)
		b.sendMessage(chatID, "Произошла ошибка при получении статистики.")
		return
	}

	// 2. Получаем топ-10 самых активных пользователей
	rows, err := b.db.Query(`
        SELECT u.username, COUNT(*) as message_count
        FROM messages m
        JOIN users u ON m.user_id = u.id
        WHERE m.chat_id = ?
        GROUP BY m.user_id
        ORDER BY message_count DESC
        LIMIT 10
    `, chatID)
	if err != nil {
		log.Printf("Ошибка получения топа пользователей: %v", err)
		b.sendMessage(chatID, "Произошла ошибка при получении статистики.")
		return
	}
	defer rows.Close()

	// Формируем сообщение со статистикой
	var statsMsg strings.Builder
	fmt.Fprintf(&statsMsg, "📊 Статистика чата:\n\n")
	fmt.Fprintf(&statsMsg, "Всего сообщений: %d\n\n", totalMessages)
	fmt.Fprintf(&statsMsg, "Топ-10 активных пользователей:\n")

	rank := 1
	for rows.Next() {
		var username string
		var count int
		if err := rows.Scan(&username, &count); err != nil {
			log.Printf("Ошибка сканирования строки: %v", err)
			continue
		}

		if username == "" {
			username = "Без username"
		}
		fmt.Fprintf(&statsMsg, "%d. %s - %d сообщ.\n", rank, username, count)
		rank++
	}

	// 3. Получаем количество сообщений за последние сутки
	var lastDayMessages int
	dayAgo := time.Now().Add(-24 * time.Hour).Unix()
	err = b.db.QueryRow(`
        SELECT COUNT(*) 
        FROM messages 
        WHERE chat_id = ? AND timestamp >= ?
    `, chatID, dayAgo).Scan(&lastDayMessages)
	if err == nil {
		fmt.Fprintf(&statsMsg, "\nСообщений за сутки: %d", lastDayMessages)
	}

	b.sendMessage(chatID, statsMsg.String())
}

// storeMessage сохраняет сообщение в истории чата
func (b *Bot) storeMessage(message *tgbotapi.Message) {
	// Пропускаем служебные сообщения
	if message.Text == "" {
		// Проверяем наличие подписи (для медиа-сообщений)
		if message.Caption == "" {
			return
		}
	}

	chatID := message.Chat.ID

	// Проверяем, может ли бот читать сообщения в этом чате
	if !b.canBotReadMessages(chatID) {
		log.Printf("Бот не может читать сообщения в чате %d", chatID)
		return
	}
	// Получаем имя пользователя
	userName := "Unknown"
	if message.From != nil {
		userName = message.From.UserName
		if userName == "" {
			userName = strings.TrimSpace(fmt.Sprintf("%s %s", message.From.FirstName, message.From.LastName))
		}
	}

	// Используем текст или подпись (для медиа-сообщений)
	text := message.Text
	if text == "" && message.Caption != "" {
		text = message.Caption
	}

	// Создаем структуру сообщения
	msg := ChatMessage{
		User: userName,
		Text: text,
		Time: time.Now(),
	}

	// Инициализируем историю чата если нужно
	if _, exists := b.chatHistories[chatID]; !exists {
		b.chatHistories[chatID] = []ChatMessage{}
	}

	// Добавляем сообщение в историю
	b.chatHistories[chatID] = append(b.chatHistories[chatID], msg)
	log.Printf("Сохранено %d: %s: %s", chatID, msg.User, msg.Text)

	// Сохраняем чат и пользователя в БД
	err := b.saveChat(message.Chat)
	if err != nil {
		log.Printf("Ошибка сохранения чата: %v", err)
	}

	if message.From != nil {
		err = b.saveUser(message.From)
		if err != nil {
			log.Printf("Ошибка сохранения пользователя: %v", err)
		}
	}

	// Сохраняем сообщение в БД
	err = b.saveMessage(
		message.Chat.ID,
		message.From.ID,
		text,
		int64(message.Date),
	)
	if err != nil {
		log.Printf("Ошибка сохранения сообщения: %v", err)
	}
}

// generateSummary создает краткую сводку с помощью локальной LLM
func (b *Bot) generateSummary(messages string) (string, error) {
	prompt := fmt.Sprintf(b.config.SummaryPrompt, messages)

	request := LocalLLMRequest{
		Model: "local-model", // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			{
				Role:    "system",
				Content: b.config.SystemPrompt,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.6,
		MaxTokens:   16000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	fmt.Println("Get AI request...")
	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
	}

	var response LocalLLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}
	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

	return response.Choices[0].Message.Content, nil
}

// generateSummary создает краткую сводку с помощью локальной LLM
func (b *Bot) generateAnekdot(messages string) (string, error) {
	prompt := fmt.Sprintf(b.config.AnekdotPrompt, messages)

	request := LocalLLMRequest{
		Model: "local-model", // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			// {
			// 	Role:    "system",
			// 	Content: b.config.SystemPrompt,
			// },
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.4,
		MaxTokens:   1000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	fmt.Println("Get AI request...")
	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
	}

	var response LocalLLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}
	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

	return response.Choices[0].Message.Content, nil
}

// cleanupOldMessages удаляет сообщения старше HistoryDays дней
func (b *Bot) cleanupOldMessages() {
	for {
		time.Sleep(1 * time.Hour) // Проверяем каждый час

		threshold := time.Now().Add(-time.Duration(b.config.HistoryDays) * 24 * time.Hour)

		for chatID, messages := range b.chatHistories {
			var filtered []ChatMessage
			for _, msg := range messages {
				if msg.Time.After(threshold) {
					filtered = append(filtered, msg)
				}
			}
			b.chatHistories[chatID] = filtered
		}
	}
}

// sendMessage отправляет сообщение в чат
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.tgBot.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}

// isChatAllowed проверяет разрешен ли чат
func (b *Bot) isChatAllowed(chatID int64) bool {
	if len(b.config.AllowedGroups) == 0 {
		return true
	}

	for _, id := range b.config.AllowedGroups {
		if id == chatID {
			return true
		}
	}
	return false
}
