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
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	godotenv "github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/natefinch/lumberjack.v2"
)

const CHECK_HOURS = -20        // hours get DB messages
const AI_REQUEST_TIMEOUT = 300 // seconds for AI request
const LIMIT_MSG = 100          //лимит сообщений запрощенных для /summary

// Config структура для конфигурации бота
type Config struct {
	TelegramToken        string
	LocalLLMUrl          string // URL локальной LLM (например "http://localhost:1234/v1/chat/completions")
	AiModelName          string
	AllowedGroups        []int64
	SummaryPrompt        string
	SystemPrompt         string
	AnekdotPrompt        string
	TopicPrompt          string
	ReplyPrompt          string
	HistoryDays          int                // Сколько дней хранить историю
	DBPath               string             // Путь к файлу SQLite
	ContextMessageLimit  int                // размер хранения контекста сообщений от пользователя
	ContextTimeLimit     int                // размер в часах хранения контекста
	ContextRetentionDays int                //удаление контекста диалога с пользователем из БД
	TokenCosts           map[string]float64 // стоимость токенов для разных моделей

}

// Bot структура основного бота
type Bot struct {
	config     Config
	tgBot      *tgbotapi.BotAPI
	httpClient *http.Client
	db         *sql.DB
	//chatHistories map[int64][]ChatMessage // История сообщений по чатам
	lastSummary map[int64]time.Time // Время последней сводки по чатам
}

// ChatMessage структура для хранения сообщений
// type ChatMessage struct {
// 	User string
// 	Text string
// 	Time time.Time
// }

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
	Model string `json:"model"`
	Usage struct {
		CompletionTokens        int `json:"completion_tokens"`
		CompletionTokensDetails struct {
			AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
			AudioTokens              int `json:"audio_tokens"`
			ReasoningTokens          int `json:"reasoning_tokens"`
			RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
		} `json:"completion_tokens_details"`
		PromptTokens        int `json:"prompt_tokens"`
		PromptTokensDetails struct {
			AudioTokens  int `json:"audio_tokens"`
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// BillingRecord представляет запись о использовании токенов AI
type BillingRecord struct {
	UserID           int64
	ChatID           int64
	Timestamp        int64
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64
}

// Разрешенные пользователи (администраторы)
var allowedAdmins = map[int64]bool{
	152657363: true, //@wrwfx
	233088195: true,
}

// Разрешенные чаты (группы, супергруппы)
var allowedChats = map[int64]bool{
	-1002478281670: true, // Атипичный чат
	-1002631108476: true, //AdminBot
	-1002407860030: true, //AdminBot2
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
		TelegramToken:        getEnv("TELEGRAM_BOT_TOKEN", ""),
		LocalLLMUrl:          getEnv("AI_LOCAL_LLM_URL", "http://localhost:1234/v1/chat/completions"),
		AiModelName:          getEnv("AI_MODEL", ""),
		AllowedGroups:        []int64{},
		HistoryDays:          30, //DB save msg days
		ContextMessageLimit:  10,
		ContextTimeLimit:     4,
		ContextRetentionDays: 7,
		DBPath:               getEnv("DB_PATH", "telegram_bot.db"),
		SummaryPrompt:        "Generate concise Russian summary of discussion. Highlight key topics. Format authors as name(@username). Use only these messages:\n%s\nReply in Russian. Sometimes mention the time hour of messages.",
		SystemPrompt:         "You're an AI assistant that creates concise Russian summaries of chat discussions. Identify main topics and essence. Always reply in Russian. Do not answer think.",
		AnekdotPrompt:        "Using these messages, create a short funny joke in Russian, loosely related to discussion. Format as one cohesive text. Don't use usernames:\n%s\nReply in Russian only.",
		TopicPrompt:          "Using these messages, create a short, funny discussion topic in Russian, loosely related to the previous conversation. Format it as one cohesive text. Add start topic question of disscussion. Do not use usernames:\n%s\nReply in Russian only.",
		ReplyPrompt:          "Create a short ansver for user question only answer if user ask it. Format it as one cohesive text. Do not use usernames:\n%s\nReply in if user ask Russian and reply another language if user ask.",
		TokenCosts: map[string]float64{
			"deepseek": 0.0001,
			"openai":   0.001,
		},
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
		config:     config,
		tgBot:      tgBot,
		httpClient: &http.Client{Timeout: AI_REQUEST_TIMEOUT * time.Second},
		db:         db,
		//chatHistories: make(map[int64][]ChatMessage),
		lastSummary: make(map[int64]time.Time),
	}, nil
}

// Run запускает бота
func (b *Bot) Run() {
	log.Printf("Бот запущен как %s", b.tgBot.Self.UserName)
	// Отправляем уведомление о запуске пользователю
	msg := tgbotapi.NewMessage(152657363, "🤖 Бот "+b.tgBot.Self.UserName+" успешно запущен!")
	_, err := b.tgBot.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения о запуске:%v", err)
	}

	// Основной цикл обработки обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.tgBot.GetUpdatesChan(u)

	// Очистка старых сообщений в БД
	go b.DeleteOldMessages()
	go b.cleanupOldContext()

	for update := range updates {
		if update.Message != nil {
			// Логирование входящего сообщения (сокращенная версия)
			logMsg := fmt.Sprintf("[%s] ", getMessageType(update.Message))

			if update.Message.From != nil {
				logMsg += fmt.Sprintf("От: @%s[%v] ", getUserName(update.Message.From), update.Message.From.ID)
			}

			if update.Message.Chat != nil {
				logMsg += fmt.Sprintf("в %s(%d) ", getChatTitle(update.Message), update.Message.Chat.ID)
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
	// Обработка команд
	if message.IsCommand() {
		b.handleCommand(message)
		return
	}

	// Проверяем, обращается ли пользователь к боту
	if b.isBotMentioned(message) {
		b.handleBotMention(message)
		return
	}

	// Обработка reply-сообщений
	if message.ReplyToMessage != nil {
		// Проверяем, является ли reply на сообщение бота
		if message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == b.tgBot.Self.ID {
			b.handleReplyToBot(message)
			return
		}
	}

	// Сохранение сообщений из групп
	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		b.storeMessage(message)
	}

}

// handleCommand обрабатывает команды бота
func (b *Bot) handleCommand(message *tgbotapi.Message) {

	if !allowedChats[message.Chat.ID] {
		b.sendMessage(message.Chat.ID, "Извините, я не работаю в этом чате.")
		return
	}

	// Проверяем, есть ли пользователь в списке разрешенных
	// if message.From != nil && !allowedUsers[message.From.ID] {
	// 	b.sendMessage(message.Chat.ID, "Не хочу выполнять вашу команду.")
	// 	return
	// }

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
		b.sendMessage(message.Chat.ID, b.getHelp())

	case "ping", "пинг":
		// Фиксируем время получения команды
		commandReceiveTime := time.Now()

		// Отправляем первый ответ
		b.sendMessage(message.Chat.ID, "pong")

		// Вычисляем время обработки
		processingTime := time.Since(commandReceiveTime)

		// Получаем время сообщения с учетом локального времени сервера
		messageTime := time.Unix(int64(message.Date), 0)
		timeDiff := time.Since(messageTime)

		// Формируем детализированный ответ
		response := fmt.Sprintf(
			"🏓 Pong!\n"+
				"⏱ Время обработки: %d ms\n"+
				"🕒 Время сервера: %s\n"+
				"⏳ Задержка сообщения: %s",
			processingTime.Milliseconds(),
			time.Now().Format("02.01.2006 15:04:05 MST"),
			formatDuration(timeDiff),
		)

		// Отправляем расширенную информацию
		b.sendMessage(message.Chat.ID, response)

	case "summary", "саммари":
		// Обработка параметра количества сообщений (по умолчанию 50)
		args := strings.Fields(message.CommandArguments())
		count := LIMIT_MSG // значение по умолчанию
		if len(args) > 0 {
			if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
				count = num
				// Ограничим максимальное количество сообщений для безопасности
				if count > LIMIT_MSG {
					count = LIMIT_MSG
					b.sendMessage(message.Chat.ID, fmt.Sprintf("Я помню только %d сообщений...", LIMIT_MSG))
				}
			}
		}
		b.handleSummaryRequest(message, count)
	// case "summary_from":
	// 	b.handleSummaryFromRequest(message)
	case "stat", "stats":
		b.handleStatsRequest(message)
	case "aistat", "aistats":
		if allowedAdmins[message.From.ID] {
			b.handleGetTopAIUsers(message)
		}
	case "anekdot", "анекдот":
		b.handleAnekdotRequest(message)
	case "tema", "topic":
		b.handleTopicRequest(message)
	case "clear", "забудь":
		b.DeleteUserContext(message.Chat.ID, message.From.ID)
	case "say", "сказать":
		// Команда для отправки сообщения от имени бота
		if allowedAdmins[message.From.ID] {
			text := message.CommandArguments()
			if text == "" {
				b.sendMessage(message.Chat.ID, "Использование: /say [текст]")
				return
			}

			// Отправляем сообщение
			b.sendMessage(message.Chat.ID, text)

			// Удаляем команду администратора
			deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
			_, err := b.tgBot.Request(deleteMsg)
			if err != nil {
				log.Printf("Не удалось удалить сообщение: %v", err)
			}
		} else {
			b.sendMessage(message.Chat.ID, "У вас нет прав.")
		}
	default:
		b.sendMessage(message.Chat.ID, "Неизвестная команда. Используйте /help для списка команд.")
	}
}

// handleBotMention обрабатывает сообщения, адресованные боту
func (b *Bot) handleBotMention(message *tgbotapi.Message) {

	// Удаляем ключевое слово или упоминание из текста
	cleanText := b.removeBotMention(message.Text)
	// Обрабатываем очищенный текст сообщения
	switch {
	case strings.Contains(strings.ToLower(cleanText), "забудь"):
		log.Println("Удаляю контекст")
		err := b.DeleteUserContext(message.Chat.ID, message.From.ID)
		if err != nil {
			log.Printf("Ошибка удаления контекста: %v", err)
			b.sendMessage(message.Chat.ID, "Не удалось очистить контекст")
			return
		}
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Все забыл =) %s", getUserName(message.From)))
	case strings.Contains(strings.ToLower(cleanText), "ping"),
		strings.Contains(strings.ToLower(cleanText), "пинг"):
		b.sendMessage(message.Chat.ID, "pong")
	case strings.Contains(strings.ToLower(cleanText), "сводка"),
		strings.Contains(strings.ToLower(cleanText), "саммари"):
		// Обработка параметра количества сообщений (по умолчанию LIMIT_MSG)
		args := strings.Fields(message.CommandArguments())
		count := LIMIT_MSG // значение по умолчанию
		if len(args) > 0 {
			if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
				count = num
				// Ограничим максимальное количество сообщений для безопасности
				if count > LIMIT_MSG {
					count = LIMIT_MSG
					b.sendMessage(message.Chat.ID, fmt.Sprintf("Я помню только %d сообщений...", LIMIT_MSG))
				}
			}
		}
		b.handleSummaryRequest(message, count)
	case strings.Contains(strings.ToLower(cleanText), "помощь"),
		strings.Contains(strings.ToLower(cleanText), "help"),
		strings.Contains(strings.ToLower(cleanText), "команды"):
		b.sendMessage(message.Chat.ID, b.getHelp())
	default:
		//b.sendMessage(message.Chat.ID, "Я вас понял, но создатель не научил меня ответить на '"+strings.ToLower(cleanText)+"'.\n\n"+b.getHelp())
		//TODO добавить отправку в AI запроса
		b.handleReplyToBot(message)
	}
}

// handleSummaryRequest обрабатывает запрос на сводку текущего чата
func (b *Bot) handleSummaryRequest(message *tgbotapi.Message, count int) {
	chatID := message.Chat.ID

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	messages, err := b.getRecentMessages(-1002478281670, count) //Выборка из БД только Атипичный Чат
	if err != nil {
		fmt.Printf("ошибка получения сообщений: %v", err)
		return
	}

	if len(messages) == 0 {
		message := fmt.Sprintf("Последние %v часов, я похоже спал =)", CHECK_HOURS*-1)
		fmt.Println(message)
		b.sendMessage(chatID, message)
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		msgTime := time.Unix(msg.Timestamp, 0)
		// Создаем часовой пояс GMT+3
		gmt3 := time.FixedZone("GMT+3", 3*60*60)
		// Переводим время сообщения в часовой пояс GMT+3
		msgTimeGMT3 := msgTime.In(gmt3)

		fmt.Fprintf(&messagesText, "[%s] %s(%v): %s\n",
			msgTimeGMT3.Format("15:04"),
			msg.UserFirstName,
			msg.Username,
			msg.Text)
	}

	//fmt.Println(messagesText.String())

	// Создание сводки с помощью локальной LLM
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.SummaryPrompt, messagesText.String()), message)

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
// func (b *Bot) handleSummaryFromRequest(message *tgbotapi.Message) {
// 	if message.ReplyToMessage == nil || message.ReplyToMessage.ForwardFromChat == nil {
// 		b.sendMessage(message.Chat.ID, "Пожалуйста, ответьте на это сообщение, переслав сообщение из чата, для которого нужно сделать сводку.")
// 		return
// 	}

// 	sourceChatID := message.ReplyToMessage.ForwardFromChat.ID
// 	history := b.chatHistories[sourceChatID]

// 	if len(history) == 0 {
// 		b.sendMessage(message.Chat.ID, fmt.Sprintf("Нет данных для чата %s.", message.ReplyToMessage.ForwardFromChat.Title))
// 		return
// 	}

// 	// Форматируем историю сообщений
// 	var messagesText strings.Builder
// 	for _, msg := range history {
// 		fmt.Fprintf(&messagesText, "[%s] %s: %s\n",
// 			msg.Time.Format("15:04"), msg.User, msg.Text)
// 	}

// 	summary, err := b.generateSummary(messagesText.String())
// 	if err != nil {
// 		log.Printf("Ошибка генерации сводки: %v", err)
// 		b.sendMessage(message.Chat.ID, "Произошла ошибка при создании сводки.")
// 		return
// 	}

// 	b.sendMessage(message.Chat.ID, fmt.Sprintf("📝 Краткая сводка из %s:\n\n%s",
// 		message.ReplyToMessage.ForwardFromChat.Title, summary))
// }

// handleGetTopAIUsers возвращает топ пользователей по использованию токенов в читаемом формате
func (b *Bot) handleGetTopAIUsers(message *tgbotapi.Message) {
	// Проверяем права пользователя (только админы могут запрашивать статистику)
	// if !b.isUserAdmin(message.Chat.ID, message.From.ID) {
	// 	b.sendMessage(message.Chat.ID, "🚫 У вас нет прав.")
	// 	return
	// }

	// Получаем топ 10 пользователей за последние 30 дней
	topUsers, err := b.GetTopUsersByTokenUsage(10, 30)
	if err != nil {
		log.Printf("Ошибка получения топ пользователей: %v", err)
		b.sendMessage(message.Chat.ID, "⚠️ Произошла ошибка при получении статистики.")
		return
	}

	if len(topUsers) == 0 {
		b.sendMessage(message.Chat.ID, "📊 Нет данных об использовании AI за последние 30 дней.")
		return
	}

	// Получаем общую статистику по чату
	chatStats, err := b.GetChatTokenUsage(message.Chat.ID, 30)
	if err != nil {
		log.Printf("Ошибка получения статистики чата: %v", err)
	}

	// Формируем красивое сообщение
	var reply strings.Builder
	reply.WriteString("📊 <b>Топ пользователей по использованию AI</b>\n")
	reply.WriteString("⏱ Период: последние 30 дней\n\n")

	// Добавляем общую статистику по чату
	if chatStats.TotalTokens > 0 {
		reply.WriteString("💬 <b>Общее по чату:</b>\n")
		reply.WriteString(fmt.Sprintf("🪙 Токены: %d (запросы: %d, ответы: %d)\n",
			chatStats.TotalTokens, chatStats.PromptTokens, chatStats.CompletionTokens))
		reply.WriteString(fmt.Sprintf("💵 Примерная стоимость: $%.2f\n\n", chatStats.Cost))
	}

	reply.WriteString("🏆 <b>Топ пользователей:</b>\n")

	for i, user := range topUsers {
		// Получаем информацию о пользователе
		username, err := b.getUserByID(user.UserID)
		if err != nil || username == nil {
			continue
		}

		// Форматируем строку для каждого пользователя
		reply.WriteString(fmt.Sprintf("%d. %s:\n", i+1, username))
		reply.WriteString(fmt.Sprintf("   🪙 Токены: %d\n", user.TotalTokens))
		reply.WriteString(fmt.Sprintf("   💵 Примерная стоимость: $%.2f\n\n", user.Cost))
	}

	// Добавляем подсказку
	//reply.WriteString("\nℹ️ Для получения детальной статистики используйте /aitokens @username")

	// Отправляем сообщение
	msg := tgbotapi.NewMessage(message.Chat.ID, reply.String())
	msg.ParseMode = "HTML"
	if _, err := b.tgBot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}

// handleSummaryRequest обрабатывает запрос на сводку текущего чата
func (b *Bot) handleAnekdotRequest(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	messages, err := b.getRecentMessages(-1002478281670, -1) //Выборка из БД только Атипичный Чат
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
			msg.UserFirstName,
			msg.Text)
	}

	// Создание сводки с помощью локальной LLM
	//summary, err := b.generateAnekdot(messagesText.String(), chatID)
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.AnekdotPrompt, messagesText.String()), message)

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

	messages, err := b.getRecentMessages(-1002478281670, -1) //Выборка из БД только Атипичный Чат
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
	//summary, err := b.generateTopic(messagesText.String(), chatID)
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.TopicPrompt, messagesText.String()), message)

	if err != nil {
		log.Printf("Ошибка генерации темы обсуждений: %v", err)
		b.sendMessage(chatID, "Не смог придумать тему, сорян.")
		return
	}

	fmt.Printf("Resp AI: %v", summary)

	b.sendMessage(chatID, "Обсудим?\n\n"+summary)
	b.lastSummary[chatID] = time.Now()
}

// handleStatsRequest показывает статистику по сообщениям и благодарностям из БД
func (b *Bot) handleStatsRequest(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Формируем сообщение со статистикой
	var statsMsg strings.Builder
	fmt.Fprintf(&statsMsg, "📊 Статистика чата:\n\n")

	// 1. Общая статистика по сообщениям
	var totalMessages int
	err := b.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&totalMessages)
	if err == nil {
		fmt.Fprintf(&statsMsg, "📨 Всего сообщений: %d\n", totalMessages)
	}

	// 2. Статистика по благодарностям
	var totalThanks int
	err = b.db.QueryRow("SELECT COUNT(*) FROM thanks WHERE chat_id = ?", chatID).Scan(&totalThanks)
	if err == nil {
		fmt.Fprintf(&statsMsg, "🙏 Всего благодарностей: %d\n\n", totalThanks)
	}

	// 3. Топ получателей благодарностей
	fmt.Fprintf(&statsMsg, "🏆 Топ-5 самых благодарных пользователей:\n")
	rows, err := b.db.Query(`
        SELECT u.username, COUNT(*) as thanks_count
        FROM thanks t
        JOIN users u ON t.from_user_id = u.id
        WHERE t.chat_id = ?
        GROUP BY t.from_user_id
        ORDER BY thanks_count DESC
        LIMIT 5
    `, chatID)
	if err == nil {
		defer rows.Close()
		rank := 1
		for rows.Next() {
			var username string
			var count int
			if err := rows.Scan(&username, &count); err != nil {
				continue
			}
			if username == "" {
				username = "Без username"
			}
			fmt.Fprintf(&statsMsg, "%d. %s - %d раз\n", rank, username, count)
			rank++
		}
	}

	// 4. Топ получателей благодарностей
	fmt.Fprintf(&statsMsg, "\n💖 Топ-5 самых ценных участников:\n")
	rows, err = b.db.Query(`
        SELECT u.username, COUNT(*) as thanks_received
        FROM thanks t
        JOIN users u ON t.to_user_id = u.id
        WHERE t.chat_id = ? AND t.to_user_id != 0
        GROUP BY t.to_user_id
        ORDER BY thanks_received DESC
        LIMIT 5
    `, chatID)
	if err == nil {
		defer rows.Close()
		rank := 1
		for rows.Next() {
			var username string
			var count int
			if err := rows.Scan(&username, &count); err != nil {
				continue
			}
			if username == "" {
				username = "Без username"
			}
			fmt.Fprintf(&statsMsg, "%d. %s - %d благодарностей\n", rank, username, count)
			rank++
		}
	}

	// 5. Последние благодарности
	fmt.Fprintf(&statsMsg, "\n🆕 Последние благодарности:\n")
	rows, err = b.db.Query(`
        SELECT u1.username, u2.username, t.text
        FROM thanks t
        LEFT JOIN users u1 ON t.from_user_id = u1.id
        LEFT JOIN users u2 ON t.to_user_id = u2.id
        WHERE t.chat_id = ?
        ORDER BY t.timestamp DESC
        LIMIT 3
    `, chatID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var fromUser, toUser, text string
			if err := rows.Scan(&fromUser, &toUser, &text); err != nil {
				continue
			}
			if fromUser == "" {
				fromUser = "Аноним"
			}
			if toUser == "" {
				toUser = "всех"
			}
			fmt.Fprintf(&statsMsg, "👉 %s → %s: %s\n", fromUser, toUser, truncateText(text, 20))
		}
	}

	// 6. Статистика за последние сутки
	dayAgo := time.Now().Add(-24 * time.Hour).Unix()
	var lastDayThanks int
	err = b.db.QueryRow(`
        SELECT COUNT(*) 
        FROM thanks 
        WHERE chat_id = ? AND timestamp >= ?
    `, chatID, dayAgo).Scan(&lastDayThanks)
	if err == nil {
		fmt.Fprintf(&statsMsg, "\n🕒 Благодарностей за сутки: %d", lastDayThanks)
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

	// Используем текст или подпись (для медиа-сообщений)
	text := message.Text
	if text == "" && message.Caption != "" {
		text = message.Caption
	}

	// // Создаем структуру сообщения
	// msg := ChatMessage{
	// 	User: userName,
	// 	Text: text,
	// 	Time: time.Now(),
	// }

	// // Инициализируем историю чата если нужно
	// if _, exists := b.chatHistories[chatID]; !exists {
	// 	b.chatHistories[chatID] = []ChatMessage{}
	// }

	// // Добавляем сообщение в историю
	// b.chatHistories[chatID] = append(b.chatHistories[chatID], msg)
	// //log.Printf("Сохранено %d: [%v]%s: %s", chatID, userID, msg.User, msg.Text)

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

	// Проверяем, содержит ли сообщение "спасибо" или "спс"
	b.checkForThanks(message)
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

// handleReplyToBot обрабатывает ответы на сообщения бота
func (b *Bot) handleReplyToBot(message *tgbotapi.Message) {
	log.Printf("Пользователь %d обратился: %s", message.From.ID, message.Text)

	// Сохраняем контекст пользователя
	err := b.saveContext(
		message.Chat.ID,
		message.From.ID,
		"user",
		message.Text,
		message.Time().Unix(),
	)
	if err != nil {
		log.Printf("Ошибка сохранения контекста: %v", err)
	}

	// Получаем историю диалога (последние 30 сообщений или за последние 24 часа)
	context, err := b.getConversationContext(
		message.Chat.ID,
		message.From.ID,
		b.config.ContextMessageLimit, // например 30
		b.config.ContextTimeLimit,    // например 24
	)
	if err != nil {
		log.Printf("Ошибка получения контекста: %v", err)
	}

	// Формируем промпт с учетом контекста
	var prompt string
	if len(context) > 0 {
		prompt = "Контекст предыдущего общения:\n"
		for _, msg := range context {
			prompt += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
		}
		prompt += "\nНовый запрос: " + message.Text
	} else {
		prompt = message.Text
	}

	log.Printf("prompt: %v", prompt)

	// Создание сводки с помощью локальной LLM
	summary, err := b.generateAiRequest(
		b.config.ReplyPrompt,
		//b.config.SystemPrompt,
		//fmt.Sprintf(b.config.ReplyPrompt, prompt),
		prompt,
		message,
	)
	if err != nil {
		log.Printf("Ошибка генерации reply: %v", err)
		b.sendMessage(message.Chat.ID, "Что-то мои мозги потекли.")
		return
	}

	// Сохраняем ответ бота в контекст
	err = b.saveContext(
		message.Chat.ID,
		message.From.ID,
		"assistant",
		summary,
		time.Now().Unix(),
	)
	if err != nil {
		log.Printf("Ошибка сохранения контекста ответа: %v", err)
	}

	fmt.Printf("Resp AI: %v", summary)
	b.sendMessage(message.Chat.ID, summary+" @"+message.From.UserName)
}

// ContextMessage представляет сообщение в контексте диалога
type ContextMessage struct {
	Role      string // "user" или "assistant"
	Content   string
	Timestamp int64
}
