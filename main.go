//return -1001225930156, nil

package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	godotenv "github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/natefinch/lumberjack.v2"
)

const CHECK_HOURS = -6         // hours get DB messages
const AI_REQUEST_TIMEOUT = 300 // seconds for AI request

// Config структура для конфигурации бота
type Config struct {
	TelegramToken string
	LocalLLMUrl   string // URL локальной LLM (например "http://localhost:1234/v1/chat/completions")
	AiModelName   string
	AllowedGroups []int64
	SummaryPrompt string
	SystemPrompt  string
	AnekdotPrompt string
	TopicPrompt   string
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
	ID            int
	ChatID        int64
	UserID        int64
	UserFirstName string
	UserLastName  string
	Text          string
	Timestamp     int64
	Username      string
	ChatTitle     string
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
	// Настройка логирования
	setupLogger()

	// Загрузка переменных окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		log.Printf("Ошибка загрузки .env файла: %v (продолжаем с переменными окружения)", err)
	}

	// Загрузка конфигурации
	config := Config{
		TelegramToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		LocalLLMUrl:   getEnv("AI_LOCAL_LLM_URL", "http://localhost:1234/v1/chat/completions"),
		AiModelName:   getEnv("AI_MODEL", ""),
		AllowedGroups: []int64{},
		//SummaryPrompt: "Создай краткую сводку обсуждения. Выдели ключевые темы обсуждения. Авторы сообщений в формате @username. Будь  информативным. Используй только эти сообщения:\n%s",
		//SystemPrompt:  "Ты полезный ассистент, который создает краткие содержательные пересказы обсуждений в чатах. Выделяющий тему и суть разговора.",
		//AnekdotPrompt: "Используя предоставленные сообщения пользователей, придумайте короткий, забавный анекдот, частично связанный с обсуждением. Напиши анекдот в виде одного законченного текста. Не используй в тексте анекдота username, придумай:\n%s",
		HistoryDays:   1,
		DBPath:        getEnv("DB_PATH", "telegram_bot.db"),
		SummaryPrompt: "Generate concise Russian summary of discussion. Highlight key topics. Format authors as name(@username). Use only these messages:\n%s\nReply in Russian.",
		SystemPrompt:  "You're an AI assistant that creates concise Russian summaries of chat discussions. Identify main topics and essence. Always reply in Russian.",
		AnekdotPrompt: "Using these messages, create a short funny joke in Russian, loosely related to discussion. Format as one cohesive text. Don't use usernames:\n%s\nReply in Russian only.",
		TopicPrompt:   "Using these messages, create a short, funny discussion topic in Russian, loosely related to the previous conversation. Format it as one cohesive text. Add start topic question of disscussion. Do not use usernames:\n%s\nReply in Russian only.",
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

func setupLogger() {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Не удалось создать директорию для логов: %v. Используется текущая директория.", err)
		logDir = "."
	}

	logFile := filepath.Join(logDir, "telegram_bot.log")

	// Настройка ротации логов
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10, // MB
		MaxBackups: 7,  // сохранять до 7 файлов
		MaxAge:     7,  // хранить до 7 дней
		Compress:   true,
		LocalTime:  true,
	}

	// Направляем вывод логов в файл и в stdout
	log.SetOutput(io.MultiWriter(os.Stdout, lumberjackLogger))

	log.Println("Логирование запущено.")
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
			// Логирование входящего сообщения (сокращенная версия)
			logMsg := fmt.Sprintf("[%s] ", getMessageType(update.Message))

			if update.Message.From != nil {
				logMsg += fmt.Sprintf("От: @%s ", getUserName(update.Message.From))
			}

			if update.Message.Chat != nil {
				logMsg += fmt.Sprintf("в %s(%d) ", getChatTitle(update.Message.Chat), update.Message.Chat.ID)
			}

			// Добавляем либо текст, либо подпись, либо отметку о медиа
			switch {
			case update.Message.Text != "":
				text := update.Message.Text
				if len(text) > 50 {
					text = text[:50] + "..."
				}
				logMsg += fmt.Sprintf("- %q", text)
			case update.Message.Caption != "":
				caption := update.Message.Caption
				if len(caption) > 50 {
					caption = caption[:50] + "..."
				}
				logMsg += fmt.Sprintf("- [подпись] %q", caption)
			default:
				logMsg += "- [медиа]"
			}

			log.Println(logMsg)

			// Обработка сообщения
			b.processMessage(update.Message)
		}
	}
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
		b.sendMessage(message.Chat.ID, "Не хочу выполнять вашу команду.")
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
	case "скучно":
		b.handleTopicRequest(message)
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
	messages, err := b.getRecentMessages(-1002478281670, 5) //Выборка из БД только Атипичный Чат
	if err != nil {
		fmt.Printf("ошибка получения сообщений: %v", err)
		return
	}

	if len(messages) == 0 {
		message := fmt.Sprintf("За последние %v часов, я похоже спал =)", CHECK_HOURS*-1)
		fmt.Println(message)
		b.sendMessage(chatID, message)
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		msgTime := time.Unix(msg.Timestamp, 0)
		fmt.Fprintf(&messagesText, "[%s] %s(%v): %s\n",
			msgTime.Format("15:04"),
			msg.UserFirstName,
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
	messages, err := b.getRecentMessages(-1002478281670, 100) //Выборка из БД только Атипичный Чат
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

// handleSummaryRequest обрабатывает запрос на сводку текущего чата
func (b *Bot) handleTopicRequest(message *tgbotapi.Message) {
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
	summary, err := b.generateTopic(messagesText.String())
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
	userID := message.From.ID

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
	log.Printf("Сохранено %d: [%v]%s: %s", chatID, userID, msg.User, msg.Text)

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
		userID,
		text,
		int64(message.Date),
	)
	if err != nil {
		log.Printf("Ошибка сохранения сообщения: %v", err)
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
