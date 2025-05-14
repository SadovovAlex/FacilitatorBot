package main

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
