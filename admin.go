package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleSay обрабатывает команду /say для отправки сообщений от имени бота
func (b *Bot) handleSay(message *tgbotapi.Message) {
	// Проверяем, является ли пользователь администратором в Telegram
	isAdmin, err := b.IsUserAdminInTelegram(message.Chat.ID, message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "Ошибка проверки прав администратора в Telegram")
		return
	}
	if !isAdmin {
		b.sendMessage(message.Chat.ID, "У вас нет прав администратора в этой группе")
		return
	}

	// Проверяем, является ли пользователь администратором в БД
	isDBAdmin, err := b.IsUserAdminInDB(message.Chat.ID, message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "Ошибка проверки прав администратора в БД")
		return
	}
	if !isDBAdmin {
		b.sendMessage(message.Chat.ID, "У вас нет прав администратора в этой группе")
		return
	}

	// Получаем текст для отправки
	text := message.CommandArguments()
	if text == "" {
		b.sendMessage(message.Chat.ID, "Использование: /say [текст]")
		return
	}

	// Отправляем сообщение
	b.sendMessage(message.Chat.ID, text)

	// Удаляем команду администратора
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
	_, err = b.tgBot.Request(deleteMsg)
	if err != nil {
		log.Printf("Не удалось удалить сообщение: %v", err)
	}
}

// IsUserAdmin проверяет, является ли пользователь администратором в Telegram или в БД
func (b *Bot) IsUserAdmin(chatID, userID int64) (bool, error) {
	// Проверяем права администратора в Telegram
	isTelegramAdmin, err := b.IsUserAdminInTelegram(chatID, userID)
	if err != nil {
		return false, fmt.Errorf("ошибка проверки прав администратора в Telegram: %v", err)
	}

	if isTelegramAdmin {
		return true, nil
	}

	// Если не админ в Telegram, проверяем в БД
	isDBAdmin, err := b.IsUserAdminInDB(chatID, userID)
	if err != nil {
		return false, fmt.Errorf("ошибка проверки прав администратора в БД: %v", err)
	}

	return isDBAdmin, nil
}

// IsUserAdminInTelegram проверяет, является ли пользователь администратором в Telegram
func (b *Bot) IsUserAdminInTelegram(chatID, userID int64) (bool, error) {
	// Получаем информацию о чате
	chatMember, err := b.tgBot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		return false, fmt.Errorf("ошибка получения информации о члене чата: %v", err)
	}

	// Проверяем статус пользователя
	return chatMember.Status == "administrator" || chatMember.Status == "creator", nil
}



// handleAIStats обрабатывает команду /aistat для получения статистики использования AI
func (b *Bot) handleAIStats(message *tgbotapi.Message) {
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

	// Получаем статистику использования токенов
	stats, err := b.GetChatTokenUsage(message.Chat.ID, 30) // Последние 30 дней
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Ошибка получения статистики: %v", err))
		return
	}

	// Формируем сообщение со статистикой
	msg := "Статистика использования AI за последний месяц:\n"
	msg += fmt.Sprintf("- Всего токенов: %d\n", stats.TotalTokens)
	msg += fmt.Sprintf("- Токенов в промптах: %d\n", stats.PromptTokens)
	msg += fmt.Sprintf("- Токенов в ответах: %d\n", stats.CompletionTokens)
	msg += fmt.Sprintf("- Стоимость: %.2f USD\n", stats.Cost)

	b.sendMessage(message.Chat.ID, msg)
}

// handleAdminCommand обрабатывает команды администраторов
func (b *Bot) handleAdminCommand(message *tgbotapi.Message) {
	switch message.Command() {
	case "say", "сказать":
		b.handleSay(message)
	case "aistat", "aistats":
		b.handleAIStats(message)
	default:
		b.handleUnknownCommand(message)
	}
}
