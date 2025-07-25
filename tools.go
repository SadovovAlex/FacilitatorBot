package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// В начале файла (или в структуре бота) добавляем массив заголовков
var summaryTitles = []string{
	"📝 **Сводка обсуждений**",
	"🔍📌 *Итоги дискуссии*\n────────────",
	"❓ *Что обсуждали?*",
	"📰 *Последние обсуждения*",
	"📌 *Кратко:*",
	"💡 *Мысли и идеи*",
	"🤔 *Рефлексия дискуссии*",
	"🎤 *Что тут наговорили?*",
	"⚙️ *Технические итоги*",
	fmt.Sprintf("⏱ *Обсуждение на %s*", time.Now().Format("15:04")),
}

// Функция для получения случайного заголовка
func getRandomSummaryTitle() string {
	rand.Seed(time.Now().UnixNano())
	return summaryTitles[rand.Intn(len(summaryTitles))]
}

// Вспомогательная функция для расчета стоимости
func calculateCost(model string, tokens int) float64 {
	// Здесь должна быть ваша логика расчета стоимости
	// Например, для GPT-4:
	if strings.Contains(model, "gpt-4") {
		return float64(tokens) * 0.00006 // примерная стоимость
	}
	return float64(tokens) * 0.000002 // для других моделей
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

// isBotMentioned проверяет, обращается ли сообщение к боту
func (b *Bot) isBotMentioned(message *tgbotapi.Message) bool {
	// Приводим текст к нижнему регистру для регистронезависимого сравнения
	lowerText := strings.ToLower(message.Text)

	// Проверяем обращения по ключевым словам
	keywords := []string{"sheriff", "шериф", "шерифф"}
	for _, kw := range keywords {
		if strings.HasPrefix(lowerText, kw) {
			return true
		}
	}

	// Проверяем прямое упоминание бота через @username
	if message.Entities != nil {
		for _, entity := range message.Entities {
			if entity.Type == "mention" {
				mention := message.Text[entity.Offset : entity.Offset+entity.Length]
				if strings.EqualFold(mention, "@"+b.tgBot.Self.UserName) {
					return true
				}
			}
		}
	}

	return false
}

// getHelp возвращает текст справки с доступными командами
func (b *Bot) getHelp() string {
	return `Доступные команды:
/help - показать это сообщение
/summary [N] - получить сводку обсуждений (N - количество сообщений, по умолчанию 100)
/anekdot - придумать анекдот по темам обсуждения
/tema - продолжить обсуждение темы
/stats - показать статистику сообщений и благодарностей
/aistats - показать статистику использования AI (только для администраторов)
/clear или /забудь - очистить контекст общения
/ping или /пинг - проверить работоспособность бота

Вы также можете обратиться ко мне напрямую:
- Начиная сообщение с "Sheriff", "Шериф" или "Шерифф"
- Или упомянув меня через @username (@` + b.tgBot.Self.UserName + `)

Примеры:
- /summary 50 - получить сводку последних 50 сообщений
- /anekdot - получить анекдот
- /stats - посмотреть статистику чата`
}

// removeBotMention удаляет упоминание бота из текста сообщения
func (b *Bot) removeBotMention(text string) string {
	lowerText := strings.ToLower(text)

	// Удаляем ключевые слова
	keywords := []string{"sheriff:", "шериф:", "шерифф:"}
	for _, kw := range keywords {
		if strings.HasPrefix(lowerText, kw) {
			return strings.TrimSpace(text[len(kw):])
		}
	}

	// Удаляем упоминание @username
	if strings.Contains(lowerText, "@"+strings.ToLower(b.tgBot.Self.UserName)) {
		return strings.ReplaceAll(text, "@"+b.tgBot.Self.UserName, "")
	}

	return text
}

// Вспомогательная функция для обрезания текста
func (b *Bot) truncateText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
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

// checkForThanks проверяет сообщение на наличие слов благодарности и сохраняет в БД
func (b *Bot) checkForThanks(message *tgbotapi.Message) {
	text := message.Text
	if text == "" && message.Caption != "" {
		text = message.Caption
	}

	lowerText := strings.ToLower(text)
	containsThanks := strings.Contains(lowerText, "спасибо") ||
		strings.Contains(lowerText, "спс ") ||
		strings.Contains(lowerText, "благодарю")

	if !containsThanks {
		return
	} else {
		fmt.Printf("спс found")
	}

	// Определяем, кому адресовано спасибо
	var thankedUserID int64 = 0
	var thankedUsername, thankedName string

	// Если это ответ на сообщение
	if message.ReplyToMessage != nil {
		thankedUserID = message.ReplyToMessage.From.ID
		thankedUsername = message.ReplyToMessage.From.UserName
		thankedName = message.ReplyToMessage.From.FirstName
	} else {
		// Попробуем найти упоминание @username в тексте
		if message.Entities != nil {
			for _, entity := range message.Entities {
				if entity.Type == "mention" {
					username := text[entity.Offset : entity.Offset+entity.Length]
					// Здесь нужно получить userID по username из БД
					user, err := b.getUserByUsername(username[1:]) // Убираем @
					if err == nil && user != nil {
						thankedUserID = user.ID
						thankedUsername = user.UserName
					}
				}
			}
		}
	}

	// Сохраняем благодарность
	err := b.saveThanks(
		message.Chat.ID,
		message.From.ID,
		thankedUserID,
		text,
		int64(message.Date),
		message.MessageID,
	)
	if err != nil {
		log.Printf("Ошибка сохранения благодарности: %v", err)
	}
	//
	// Формируем ответное сообщение
	response := tgbotapi.NewMessage(message.Chat.ID, "")
	response.ReplyToMessageID = message.MessageID

	// Добавляем текст
	thanksText := ""
	if thankedUserID != 0 {
		thanksText = fmt.Sprintf("🔥 %s, благодарность улетает %s (@%s) !\n", message.From.FirstName, thankedName, thankedUsername)
	} else {
		thanksText = fmt.Sprintf("🔥 %s, благодарность улетает в космос! Вероятно какому то пользователю, но ты не ответил на сообщение пользователя словом `спасибо`.\n", message.From.FirstName)
	}

	// Добавляем статистику
	var stats strings.Builder
	stats.WriteString("\n📊 Статистика благодарностей:\n")

	// 1. Общее количество благодарностей отправителя
	var userThanksCount int
	err = b.db.QueryRow("SELECT COUNT(*) FROM thanks WHERE from_user_id = ? AND chat_id = ?",
		message.From.ID, message.Chat.ID).Scan(&userThanksCount)
	if err == nil {
		fmt.Fprintf(&stats, "Ты сказал спасибо %d раз(а)\n", userThanksCount)
	}

	// Если благодарили конкретного пользователя, показываем его статистику и место в топе
	if thankedUserID != 0 {
		var thankedCount int
		err = b.db.QueryRow("SELECT COUNT(*) FROM thanks WHERE to_user_id = ? AND chat_id = ?",
			thankedUserID, message.Chat.ID).Scan(&thankedCount)
		if err == nil {
			if thankedUsername != "" {
				fmt.Fprintf(&stats, "Всего поблагодарили %s (@%s) %d раз(а)\n", thankedName, thankedUsername, thankedCount)
			} else {
				fmt.Fprintf(&stats, "Всего поблагодарили этого пользователя %d раз(а)\n", thankedCount)
			}

			// Получаем место в топе получателей благодарностей
			var rank int
			err = b.db.QueryRow(`
			SELECT position FROM (
				SELECT 
					to_user_id, 
					RANK() OVER (ORDER BY COUNT(*) DESC) as position
				FROM thanks 
				WHERE chat_id = ?
				GROUP BY to_user_id
			) ranked WHERE to_user_id = ?`,
				message.Chat.ID, thankedUserID).Scan(&rank)

			if err == nil {
				if rank <= 5 {
					fmt.Fprintf(&stats, "🏆 Место в топе получателей: %d\n", rank)
				} else {
					fmt.Fprintf(&stats, "🏆 В топ-5 не входит (место: %d)\n", rank)
				}
			}
		}
	}

	thanksText += stats.String()
	response.Text = thanksText
	response.ChatID = message.Chat.ID

	// // Добавляем стикер (можно использовать ID стикера или отправить картинку)
	// sticker := tgbotapi.NewSticker(message.Chat.ID, tgbotapi.FileID("CAACAgIAAxkBAAIB..." /* замените на реальный ID стикера */))
	// b.sendSticker(sticker)

	// // Отправляем текстовое сообщение
	//b.sendMessage(message.Chat.ID , response)
	b.tgBot.Send(response)
	//

}

// Вспомогательная функция для получения названия чата
func getChatTitle(message *tgbotapi.Message) string {
	if message.Chat == nil {
		return "Unknown"
	}

	switch message.Chat.Type {
	case "group", "supergroup":
		if message.Chat.Title != "" {
			return message.Chat.Title
		}
		return "Group Chat"
	case "private":
		return getUserName(message.From)
	case "channel":
		if message.Chat.Title != "" {
			return message.Chat.Title
		}
		return "Channel"
	default:
		return "Unknown"
	}
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

// getUserByID получает пользователя по ID из БД
func (b *Bot) getUserByID(userID int64) (*tgbotapi.User, error) {
	var user tgbotapi.User
	err := b.db.QueryRow(`
        SELECT id, username, first_name, last_name
        FROM users
        WHERE id = ?`, userID).Scan(
		&user.ID, &user.UserName, &user.FirstName, &user.LastName)

	if err != nil {
		return nil, err
	}
	return &user, nil
}

// formatDuration форматирует duration в читаемый вид
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1f сек", d.Seconds())
	}

	return fmt.Sprintf("%d мин %d сек", int(d.Minutes()), int(d.Seconds())%60)
}

// Вспомогательная функция для определения типа сообщения
func getMessageType(msg *tgbotapi.Message) string {
	switch {
	case msg.Text != "":
		return "текст"
	case msg.Photo != nil:
		return "фото"
	case msg.Video != nil:
		return "видео"
	case msg.Document != nil:
		return "документ"
	case msg.Audio != nil:
		return "аудио"
	case msg.Voice != nil:
		return "голосовое"
	case msg.Sticker != nil:
		return "стикер"
	case msg.Location != nil:
		return "локация"
	case msg.Contact != nil:
		return "контакт"
	case msg.Animation != nil:
		return "гифка"
	default:
		return "сообщение"
	}
}

// startChatTyping запускает индикатор печати в чате
func (b *Bot) startChatTyping(chatID int64) chan struct{} {
	// Отправляем индикатор печати сразу при запуске
	chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	log.Printf("[startChatTyping] Отправка индикатора печати.")
	if _, err := b.tgBot.Request(chatAction); err != nil {
		log.Printf("[startChatTyping] Ошибка отправки индикатора печати: %v", err)
		return nil
	}

	stopTyping := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
				if _, err := b.tgBot.Request(chatAction); err != nil {
					log.Printf("[startChatTyping] Ошибка отправки индикатора печати: %v", err)
				}
			case <-stopTyping:
				return
			}
		}
	}()
	return stopTyping // Возвращаем канал для остановки
}
