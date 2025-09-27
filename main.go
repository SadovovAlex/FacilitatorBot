//return -1001225930156, nil

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"facilitatorbot/db"
	"facilitatorbot/module"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	godotenv "github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/natefinch/lumberjack.v2"
)

const AI_REQUEST_TIMEOUT = 180 // seconds for AI request
const LIMIT_MSG = 100          //default лимит сообщений запрощенных для /summary
const IGNORE_OLD_MSG_MIN = 15  // игнорируем старые сообщение если не прочитали, но пишем в БД все =)
const LOG_FILENAME = "tg_bot.log"
const LOG_DIR = "logs"

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
	ImagePrompt          string
	HistoryDays          int                // Сколько дней хранить историю
	DBPath               string             // Путь к файлу SQLite
	ContextMessageLimit  int                // размер хранения контекста сообщений от пользователя
	ContextTimeLimit     int                // размер в часах хранения контекста
	ContextRetentionDays int                //удаление контекста диалога с пользователем из БД
	TokenCosts           map[string]float64 // стоимость токенов для разных моделей
	AIImageURL           string             // URL для генерации изображений
}

// Bot структура основного бота
type Bot struct {
	config         Config
	tgBot          *tgbotapi.BotAPI
	httpClient     *http.Client
	db             *db.DB
	captchaManager *module.CaptchaManager
	//chatHistories map[int64][]ChatMessage // История сообщений по чатам
	lastSummary map[int64]time.Time // Время последней сводки по чатам
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

func main() {
	// Настройка логирования
	setupLogger()

	// Логируем информацию о версии
	if BuildDate != "" {
		log.Printf("Init %s Дата сборки: %s", Version, BuildDate)
	} else {
		log.Printf("Init %s", Version)
	}

	// Загрузка переменных окружения из .env файла
	log.Printf(".env loading...")
	err := godotenv.Load()
	if err != nil {
		log.Printf("Ошибка загрузки .env файла: %v (продолжаем с переменными окружения)", err)
	}

	// Загрузка конфигурации
	config := Config{
		TelegramToken:        getEnv("TELEGRAM_BOT_TOKEN", ""),
		LocalLLMUrl:          getEnv("AI_LOCAL_LLM_URL", "http://localhost:1234/v1/chat/completions"),
		AiModelName:          getEnv("AI_MODEL", ""),
		AllowedGroups:        parseAllowedGroups(getEnv("ALLOWED_GROUPS", "")),
		HistoryDays:          30, //DB save msg days
		ContextMessageLimit:  10,
		ContextTimeLimit:     4,
		ContextRetentionDays: 7,
		DBPath:               getEnv("DB_PATH", "telegram_bot.db"),
		AIImageURL:           getEnv("AI_IMAGE_URL", "https://image.pollinations.ai/prompt/"),
		SummaryPrompt:        "Generate concise Russian summary of discussion. Highlight key topics. Format authors as name(@username). Use only these messages:\n%s\nReply in Russian. Sometimes mention the time hour of messages.",
		SystemPrompt:         "You're an AI assistant that creates concise Russian summaries of chat discussions. Identify main topics and essence. Always reply in Russian. Do not answer think.",
		AnekdotPrompt:        "Using these messages, create a short funny joke in Russian, loosely related to discussion. Format as one cohesive text. Don't use usernames:\n%s\nReply in Russian only.",
		TopicPrompt:          "Using these messages, create a short, funny discussion topic in Russian, loosely related to the previous conversation. Format it as one cohesive text. Add start topic question of disscussion. Do not use usernames:\n%s\nReply in Russian only.",
		//ReplyPrompt:          "Create a ansver for user question. Format it as one cohesive text. Do not use usernames:\n%s\nReply in if user ask Russian and reply another language if user ask.",
		ReplyPrompt: `Ты — AI-собеседник по имени "Шерифф". Твой стиль общения: дружелюбный, вежливый, поддерживающий и немного разговорный. Ты стремишься быть максимально полезным, даешь подробные и обоснованные ответы, а также проявляешь искренний интерес к диалогу.
Критически важные инструкции для каждого твоего ответа:
1. **Язык:** Всегда отвечай на том же языке, на котором пользователь написал свое сообщение. Не переключай языки произвольно.
2. **Формат ответа:** Ответ должен быть единым, связным и хорошо структурированным текстом. Не используй маркеры списка (например, - / *), если об этом не попросили явно.
3. **Обращения:** Не используй в ответе username'ы (например, "Пользователь:", "Дорогой пользователь" и т.д.). Веди диалог так, как будто это естественная беседа.
4. **Участие:** Поддержи беседу. Если уместно, задай уточняющий или встречный вопрос, чтобы диалог продолжался.
5. **Без предупреждений:** Не начинай ответ с таких фраз, как "Как AI, я...", "Я не человек, но...". Просто дай лучший возможный ответ.

Проанализируй последнее сообщение пользователя и продолжай диалог:
"%s"`,
		ImagePrompt: "A cartoonish атипичный black wolf with big, expressive eyes and sharp teeth, dynamically posing while holding random objects. The wolf looks slightly confused or nervous. Simple gray background with subtle rain streaks. Stylized as a humorous comic—flat colors, bold outlines, exaggerated expressions. Add top right copyright eng text `(с)wrwfx`,",
		TokenCosts: map[string]float64{
			"deepseek": 0.0001,
			"openai":   0.001,
		},
	}

	// Проверка обязательных переменных
	if config.TelegramToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен")
	}
	log.Printf("config.TelegramToken: %s***%s\n",
		config.TelegramToken[:6],
		config.TelegramToken[len(config.TelegramToken)-4:])

	// Инициализация бота
	log.Printf("TG init...")
	bot, err := NewBot(config)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}

	// Инициализация БД
	log.Printf("DB init...")
	err = bot.db.Init()
	if err != nil {
		log.Fatalf("Ошибка инициализации DB: %v", err)
	}
	defer bot.db.Close()

	// Инициализация менеджера капчи
	log.Printf("Инициализация модуля капчи...")
	bot.initializeCaptchaManager()

	// Запуск бота
	log.Printf("Запуск обработки...")
	bot.Run()
}

// initializeCaptchaManager инициализирует менеджер капчи
func (b *Bot) initializeCaptchaManager() {
	b.captchaManager = module.NewCaptchaManager(b.db.GetSQLDB())
	log.Printf("Менеджер капчи инициализирован")
}

func setupLogger() {
	logDir := LOG_DIR
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Не удалось создать директорию для логов: %v. Используется текущая директория.", err)
		logDir = "."
	}

	logFile := filepath.Join(logDir, LOG_FILENAME)

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
	log.Println("Logger Run.")
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

	dbInstance, err := db.NewDB(config.DBPath, config.HistoryDays, config.ContextRetentionDays)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания DB: %v", err)
	}

	return &Bot{
		config:      config,
		tgBot:       tgBot,
		httpClient:  &http.Client{Timeout: AI_REQUEST_TIMEOUT * time.Second},
		db:          dbInstance,
		lastSummary: make(map[int64]time.Time),
	}, nil
}

// Run запускает бота
func (b *Bot) Run() {
	// Логируем информацию о версии
	log.Printf("Бот запущен как %s, %s, %s", b.tgBot.Self.UserName, Version, BuildDate)

	// Отправляем уведомление о запуске пользователю с информацией о версии
	versionInfo := Version
	if BuildDate != "" {
		versionInfo += ", сборка: " + BuildDate
	}
	msg := tgbotapi.NewMessage(152657363, "🤖 Бот "+b.tgBot.Self.UserName+" запущен! Версия: "+versionInfo)
	_, err := b.tgBot.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения о запуске:%v", err)
	}

	// Основной цикл обработки обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.tgBot.GetUpdatesChan(u)

	// Очистка старых сообщений в БД
	go b.db.DeleteOldMessages()
	go b.db.CleanupOldContext()

	for update := range updates {
		if update.Message != nil {
			// Логирование входящего сообщения (сокращенная версия)
			logMsg := fmt.Sprintf("[Run()] Тип: %s", getMessageType(update.Message))

			if update.Message.From != nil {
				logMsg += fmt.Sprintf("%s[%v] ", getUserName(update.Message.From), update.Message.From.ID)
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
			b.processAllMessage(update.Message)
		}
	}
}

func (b *Bot) processAllMessage(message *tgbotapi.Message) {
	// Пропускаем служебные сообщения и сообщения от каналов
	if message.Text == "" || message.From == nil {
		log.Printf("Служебное: %v", message)
		return
	}

	// проверяем разрешен ли чат в .env
	if !b.isChatAllowed(message.Chat.ID) {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Извините, я не работаю в этом чате. обратитесь к администратору. %v", message.Chat.ID))
		return
	}
	// Проверяем, может ли бот читать сообщения в этом чате
	if !b.canBotReadMessages(message.Chat.ID) {
		log.Printf("Бот не может читать сообщения в чате %d", message.Chat.ID)
		return
	}

	// Сохранение сообщений из групп
	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		b.storeMessage(message)
	}

	// Проверяем возраст сообщения
	messageTime := time.Unix(int64(message.Date), 0)
	if time.Since(messageTime) > IGNORE_OLD_MSG_MIN*time.Minute {
		log.Printf("[processMessage] Old msg от %v в чате %v. Возраст: %v", getUserName(message.From), getChatTitle(message), time.Since(messageTime))
		return
	}

	// Логируем информацию о сообщении
	//log.Printf("[processMessage] Msg от %v в чате %v: %q", getUserName(message.From), getChatTitle(message), message.Text)

	// Обработка всех сообщений, валидации проверки, антиспам, капча
	b.handleAllMessages(message)

	// Обработка сообщений команд
	if message.IsCommand() {
		log.Printf("[processMessage]Команда: %s", message.Command())
		b.handleCommand(message)
		return
	}

	// Проверяем, обращается ли пользователь к боту
	if b.isBotMentioned(message) {
		log.Printf("[processMessage]Обращение к боту")
		b.handleBotMention(message)
		return
	}

	// Обработка reply-сообщений
	if message.ReplyToMessage != nil {
		log.Printf("[processMessage] Reply")
		// Проверяем, является ли reply на сообщение бота
		if message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == b.tgBot.Self.ID {
			log.Printf("[processMessage] Reply на сообщение бота")
			b.handleReplyToBot(message)
			return
		}
	}

}

func (b *Bot) storeMessage(message *tgbotapi.Message) {
	//chatID := message.Chat.ID
	userID := message.From.ID

	// Логируем ID чата и пользователя
	//log.Printf("[storeMessage] от %d в чате %d",  userID, chatID)

	// Пропускаем служебные пустые сообщения
	if message.Text == "" {
		// Проверяем наличие подписи (для медиа-сообщений)
		if message.Caption == "" {
			return
		}
	}

	// Используем текст или подпись (для медиа-сообщений)
	text := message.Text
	if text == "" && message.Caption != "" {
		text = message.Caption
	}

	// Сохраняем чат и пользователя в БД
	err := b.db.SaveChat(message.Chat)
	if err != nil {
		log.Printf("Ошибка сохранения чата: %v", err)
	}

	if message.From != nil {
		err = b.db.SaveUser(message)
		if err != nil {
			log.Printf("Ошибка сохранения пользователя: %v", err)
		}
	}

	// Сохраняем сообщение в БД
	err = b.db.SaveMessage(
		message.Chat.ID,
		userID,
		text,
		int64(message.Date),
	)
	if err != nil {
		log.Printf("Ошибка сохранения сообщения: %v", err)
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
		err := b.db.DeleteUserContext(message.Chat.ID, message.From.ID)
		if err != nil {
			log.Printf("Ошибка удаления контекста: %v", err)
			b.sendMessage(message.Chat.ID, "Не удалось очистить контекст")
			return
		}
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Все забыл =) %s", getUserName(message.From)))
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
		b.handleAISummary(message, count)
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

// handleReplyToBot обрабатывает ответы на сообщения бота
func (b *Bot) handleReplyToBot(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	log.Printf("Пользователь %d обратился: %s", message.From.ID, message.Text)

	// Запускаем горутину для периодической отправки индикатора печати
	stopTyping := b.startChatTyping(chatID)
	defer close(stopTyping)

	// Получаем системный промпт пользователя
	aiInfo, err := b.db.GetUserAIInfo(message.From.ID)
	if err != nil {
		log.Printf("Ошибка получения AI info: %v", err)
		aiInfo = "" // Используем пустой промпт по умолчанию
	}

	// Сохраняем контекст пользователя
	err = b.db.SaveContext(
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
	context, err := b.db.GetConversationContext(
		message.Chat.ID,
		message.From.ID,
		b.config.ContextMessageLimit, // например 30
		b.config.ContextTimeLimit,    // например 24
	)
	if err != nil {
		log.Printf("Ошибка получения контекста: %v", err)
	}

	// Обрабатываем сообщение с учетом системного промпта пользователя
	if aiInfo != "" {
		// Добавляем системный промпт в начало контекста
		context = append([]db.ContextMessage{{
			Role:      "system",
			Content:   aiInfo,
			Timestamp: message.Time().Unix(),
		}}, context...)
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
	err = b.db.SaveContext(
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
