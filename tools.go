package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
/help
/summary - получить сводку обсуждений
/stats - статистика по сохраненным сообщениям
/anekdot - придумаю анекдот по темам обсуждения =)
/tema - продолжим обсуждать тему

Также вы можете обратиться ко мне напрямую:
- Начиная сообщение с "Sheriff", "Шериф" или "Шерифф"
- Или упомянув меня через @username (@` + b.tgBot.Self.UserName + `)`
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
func truncateText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
}

// checkForThanks проверяет сообщение на наличие слов благодарности и сохраняет в БД// checkForThanks проверяет сообщение на наличие слов благодарности
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

	// Если это ответ на сообщение
	if message.ReplyToMessage != nil {
		thankedUserID = message.ReplyToMessage.From.ID
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
