package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleAllMessages(message *tgbotapi.Message) {

	// Проверка на спам перед обработкой команды
	if b.isSpam(message.Text) {
		b.handleSpamMessage(message)
		return
	}

	// Проверяем, содержит ли сообщение "спасибо" или "спс"
	b.checkForThanks(message)
}

// CommandHandler обрабатывает команды бота
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	switch message.Command() {
	case "start":
		b.handleStart(message)
	case "help":
		b.handleHelp(message)
	case "ping", "пинг":
		b.handlePing(message)
	case "summary", "саммари":
		b.handleAISummary(message, 0)
	case "stat", "stats":
		b.handleStats(message)
	case "aistat", "aistats":
		b.handleAdminCommand(message)
		return
	case "anekdot", "анекдот":
		b.handleAnekdot(message)
	case "tema", "topic":
		b.handleTopic(message)
	case "clear", "забудь":
		b.handleClear(message)
	case "say", "сказать":
		b.handleAdminCommand(message)
		return
	case "img":
		b.handleGenImage(message)
	default:
		b.handleUnknownCommand(message)
	}
}

// handleStart обрабатывает команду /start
func (b *Bot) handleStart(message *tgbotapi.Message) {
	b.sendMessage(message.Chat.ID, "Привет! Я бот для создания кратких пересказов обсуждений. Используй /summary для получения сводки.")
}

// handleHelp обрабатывает команду /help
func (b *Bot) handleHelp(message *tgbotapi.Message) {
	b.sendMessage(message.Chat.ID, b.getHelp())
}

// handlePing обрабатывает команду /ping
func (b *Bot) handlePing(message *tgbotapi.Message) {
	commandReceiveTime := time.Now()
	processingTime := time.Since(commandReceiveTime)
	messageTime := time.Unix(int64(message.Date), 0)
	timeDiff := time.Since(messageTime)

	response := fmt.Sprintf(
		"🏓 Pong!\n"+
			"⏱ Время обработки: %d ms\n"+
			"🕒 Время сервера: %s\n"+
			"⏳ Задержка сообщения: %s",
		processingTime.Milliseconds(),
		time.Now().Format("02.01.2006 15:04:05 MST"),
		formatDuration(timeDiff),
	)

	b.sendMessage(message.Chat.ID, response)
}

// handleSummary обрабатывает команду /summary
func (b *Bot) handleAISummary(message *tgbotapi.Message, count int) {
	chatID := message.Chat.ID

	// Запускаем горутину для периодической отправки индикатора печати
	stopTyping := b.startChatTyping(chatID)
	defer close(stopTyping)

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	if count == 0 {
		count = LIMIT_MSG
	}

	args := strings.Fields(message.CommandArguments())
	if len(args) > 0 {
		if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
			count = num
			if count > LIMIT_MSG {
				count = LIMIT_MSG
				b.sendMessage(message.Chat.ID, fmt.Sprintf("Я помню только %d сообщений...", LIMIT_MSG))
			}
		}
	}

	messages, err := b.getRecentMessages(chatID, count)
	if err != nil {
		log.Printf("[handleSummary] Ошибка получения сообщений: %v", err)
		b.sendMessage(chatID, "Не удалось получить историю сообщений.")
		return
	}

	if len(messages) == 0 {
		message := fmt.Sprintf("Последние %v часов, я похоже спал =)", CHECK_HOURS*-1)
		log.Println(message)
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

		// Форматируем и добавляем сообщение в буфер
		messagesText.WriteString(fmt.Sprintf("[%s] %s(%v): %s\n",
			msgTimeGMT3.Format("15:04"),
			msg.UserFirstName,
			msg.Username,
			msg.Text))

		// // Логируем сообщение
		// log.Printf("[%s] %s(%v): %s",
		// 	msgTimeGMT3.Format("15:04"),
		// 	msg.UserFirstName,
		// 	msg.Username,
		// 	msg.Text)
	}

	// Создание сводки с помощью локальной LLM
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.SummaryPrompt, messagesText.String()), message)
	if err != nil {
		log.Printf("[handleSummary] Ошибка генерации сводки: %v", err)
		b.sendMessage(chatID, "Не удалось сгенерировать сводку обсуждений.")
		return
	}

	b.sendMessage(chatID, getRandomSummaryTitle()+"\n"+summary)
	b.lastSummary[chatID] = time.Now()

	// Генерируем изображение на основе сводки
	//description := b.config.ImagePrompt + "\n" + summary
	description := summary

	photo, err := b.GenerateImage(description, chatID, false)
	if err != nil {
		// Если не удалось сгенерировать изображение, отправляем текст
		log.Printf("[handleSummary] Ошибка генерации изображения: %v", err)
		return
	}

	// Отправляем изображение с кратким описанием
	photo.Caption = ""
	b.tgBot.Send(photo)
	b.lastSummary[chatID] = time.Now()
}

// handleClear обрабатывает команду /clear
func (b *Bot) handleClear(message *tgbotapi.Message) {
	b.DeleteUserContext(message.Chat.ID, message.From.ID)
}

// Обработка спам-сообщений
func (b *Bot) handleSpamMessage(message *tgbotapi.Message) {
	// Константы сообщений
	const (
		adminWarning = `⚠️ *СПАМ-алерт* в [чате](https://t.me/c/%s/%d) %s
От: @%s (%s %s)
Текст сообщения:
%s`
		userWarning = `🚫 @%s, ваше сообщение будет удалено как спам!
Повторные нарушения могут привести к ограничениям.`
	)

	// Получаем список администраторов чата
	admins, err := b.tgBot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: message.Chat.ID}})
	if err != nil {
		log.Printf("Ошибка получения администраторов: %v", err)
		return
	}

	// Формируем ссылку на сообщение
	chatIDStr := fmt.Sprintf("%d", message.Chat.ID)
	if message.Chat.ID < 0 {
		chatIDStr = fmt.Sprintf("%d", message.Chat.ID*-1)
	}

	// Формируем и отправляем предупреждение админам
	warnMsg := fmt.Sprintf(adminWarning,
		chatIDStr[4:],
		message.MessageID,
		message.Chat.Title,
		message.From.UserName,
		message.From.FirstName,
		message.From.LastName,
		message.Text)

	for _, admin := range admins {
		msg := tgbotapi.NewMessage(admin.User.ID, warnMsg)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true
		_, err := b.tgBot.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки предупреждения админу %d: %v", admin.User.ID, err)
		}
	}

	// Формируем и отправляем предупреждение пользователю с упоминанием
	userWarningText := fmt.Sprintf(userWarning, message.From.UserName)
	userMsg := tgbotapi.NewMessage(message.Chat.ID, userWarningText)
	userMsg.ReplyToMessageID = message.MessageID

	// Если у пользователя есть username, делаем упоминание
	if message.From.UserName != "" {
		userMsg.Entities = []tgbotapi.MessageEntity{
			{
				Type:   "mention",
				Offset: 0,
				Length: len(message.From.UserName) + 1, // +1 для символа @
				User:   &tgbotapi.User{ID: message.From.ID},
			},
		}
	}

	_, err = b.tgBot.Send(userMsg)
	if err != nil {
		log.Printf("Ошибка отправки предупреждения пользователю: %v", err)
	}

	// Удаляем спам-сообщение
	// _, err = b.tgBot.Send(tgbotapi.DeleteMessageConfig{
	// 	ChatID:    message.Chat.ID,
	// 	MessageID: message.MessageID,
	// })
	// if err != nil {
	// 	log.Printf("Ошибка удаления сообщения: %v", err)
	// }

	// Логируем событие в БД
	go func(msg *tgbotapi.Message) {
		err := b.LogIncident(msg.Chat.ID, msg.From.ID, msg.Text, time.Now().Unix())
		if err != nil {
			log.Printf("Ошибка логирования спама: %v", err)
		}
	}(message)
}

// handleUnknownCommand обрабатывает неизвестные команды
func (b *Bot) handleUnknownCommand(message *tgbotapi.Message) {
	// Список случайных ответов
	responses := []string{
		"Такое не знаю.",
		"Извините, но эта команда мне не знакома.",
		"Не могу понять, что вы от меня хотите.",
		"Хм, не могу найти такую команду в своем меню.",
		"К сожалению, эта функция находится в разработке.",
	}

	// Инициализируем рандомайзер с текущим временем
	//rand.Seed(time.Now().UnixNano())

	// Выбираем случайный ответ
	response := responses[rand.Intn(len(responses))]

	b.sendMessage(message.Chat.ID, response)
}

// handleMem обрабатывает команду /mem
func (b *Bot) handleGenImage(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Проверяем, является ли пользователь администратором
	isAdmin, err := b.IsUserAdmin(message.Chat.ID, message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "Ошибка проверки прав администратора")
		return
	}
	if !isAdmin {
		b.sendMessage(message.Chat.ID, "У вас нет прав администратора в этой группе")
		return
	}

	// Запускаем горутину для периодической отправки индикатора печати
	stopTyping := b.startChatTyping(chatID)
	defer close(stopTyping)

	// Получаем описание из текста сообщения после команды
	description := strings.TrimSpace(message.CommandArguments())
	if description == "" {
		b.sendMessage(chatID, "Пожалуйста, укажите описание для изображения после команды /img")
		return
	}

	// // Создание промпта для генерации картинки с помощью LLM
	// promptImg, err := b.generateAiRequest("ты иллюстратор рисующий A cartoonish black wolf with big, expressive eyes and sharp teeth, dynamically posing while holding random objects (e.g., a coffee cup, umbrella, or sandwich). The wolf looks slightly confused or nervous. Simple gray background with subtle rain streaks. Stylized as a humorous comic—flat colors, bold outlines, exaggerated expressions. Footer: small copyright text (с)wrwfx in English. ",
	// 	"Сгенерируй промпт для AI по генерации картинки по теме:"+description, message)
	// if err != nil {
	// 	log.Printf("[handleGenImage] Ошибка генерации: %v", err)
	// 	b.sendMessage(chatID, "Не удалось. Попробуйте позднее.")
	// 	return
	// }
	// log.Println("[handleGenImage]" + promptImg)

	// Генерируем изображение
	//photo, err := b.GenerateImage(b.config.ImagePrompt, chatID, false)
	photo, err := b.GenerateImage(description, chatID, false)
	if err != nil {
		log.Printf("Ошибка генерации изображения: %v", err)
		b.sendMessage(chatID, "Не удалось сгенерировать изображение. Попробуйте позднее.")
		return
	}

	// Отправляем изображение
	_, err = b.tgBot.Send(*photo)
	if err != nil {
		log.Printf("Ошибка отправки изображения: %v", err)
		b.sendMessage(chatID, "Не удалось отправить изображение. Попробуйте снова.")
	}
}

// handleTopic обрабатывает команду /tema
func (b *Bot) handleTopic(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	messages, err := b.getRecentMessages(chatID, -1)
	if err != nil {
		log.Printf("Ошибка получения сообщений: %v", err)
		b.sendMessage(chatID, "Не удалось получить историю сообщений.")
		return
	}

	if len(messages) == 0 {
		b.sendMessage(chatID, "Нет сообщений для анализа.")
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&messagesText, "%s: %s\n",
			msg.Username,
			msg.Text)
	}

	// Создание темы с помощью локальной LLM
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.TopicPrompt, messagesText.String()), message)
	if err != nil {
		log.Printf("Ошибка генерации темы: %v", err)
		b.sendMessage(chatID, "Не удалось сгенерировать тему.")
		return
	}

	b.sendMessage(chatID, "Обсудим?\n\n"+summary)
	b.lastSummary[chatID] = time.Now()
}

// handleAnekdot обрабатывает команду /anekdot
func (b *Bot) handleAnekdot(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Проверка разрешен ли чат
	if !b.isChatAllowed(chatID) {
		b.sendMessage(chatID, "Извините, у меня нет доступа к истории этого чата.")
		return
	}

	messages, err := b.getRecentMessages(chatID, -1)
	if err != nil {
		log.Printf("Ошибка получения сообщений: %v", err)
		b.sendMessage(chatID, "Не удалось получить историю сообщений.")
		return
	}

	if len(messages) == 0 {
		b.sendMessage(chatID, "Нет сообщений для анализа.")
		return
	}

	// Форматируем историю сообщений
	var messagesText strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&messagesText, "%s: %s\n",
			msg.Username,
			msg.Text)
	}

	// Создание анекдота с помощью локальной LLM
	summary, err := b.generateAiRequest(b.config.SystemPrompt, fmt.Sprintf(b.config.AnekdotPrompt, messagesText.String()), message)
	if err != nil {
		log.Printf("Ошибка генерации анекдота: %v", err)
		b.sendMessage(chatID, "Не смог придумать анекдот, попробуй позже.")
		return
	}

	b.sendMessage(chatID, "📝 Аnekdot:\n\n"+summary)
	b.lastSummary[chatID] = time.Now()
}

// handleStats обрабатывает команду /stats
func (b *Bot) handleStats(message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Формируем сообщение со статистикой
	var statsMsg strings.Builder
	fmt.Fprintf(&statsMsg, "📊 Статистика чата:\n\n")

	// // 1. Общая статистика по сообщениям
	// var totalMessages int
	// err := b.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&totalMessages)
	// if err == nil {
	// 	fmt.Fprintf(&statsMsg, "📨 Всего сообщений: %d\n", totalMessages)
	// }

	// 2. Статистика по благодарностям
	var totalThanks int
	err := b.db.QueryRow("SELECT COUNT(*) FROM thanks WHERE chat_id = ?", chatID).Scan(&totalThanks)
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
			GROUP BY u.id
			ORDER BY thanks_count DESC
			LIMIT 5`, chatID)
	if err == nil {
		defer rows.Close()
		for i := 1; rows.Next(); i++ {
			var username string
			var count int
			if err := rows.Scan(&username, &count); err == nil {
				fmt.Fprintf(&statsMsg, "%d. %s (%d благодарностей)\n", i, username, count)
			}
		}
	}

	// 4. Топ получателей благодарностей
	fmt.Fprintf(&statsMsg, "\n🏆 Топ-5 самых благодаримых пользователей:\n")
	rows, err = b.db.Query(`
			SELECT u.username, COUNT(*) as thanks_count
			FROM thanks t
			JOIN users u ON t.to_user_id = u.id
			WHERE t.chat_id = ?
			GROUP BY u.id
			ORDER BY thanks_count DESC
			LIMIT 5`, chatID)
	if err == nil {
		defer rows.Close()
		for i := 1; rows.Next(); i++ {
			var username string
			var count int
			if err := rows.Scan(&username, &count); err == nil {
				fmt.Fprintf(&statsMsg, "%d. %s (%d благодарностей)\n", i, username, count)
			}
		}
	}

	b.sendMessage(chatID, statsMsg.String())
}
