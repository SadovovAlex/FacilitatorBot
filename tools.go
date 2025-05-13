package main

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
