package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Разрешенные пользователи (администраторы)
var allowedAdmins = map[int64]bool{
	152657363: true, //@wrwfx
	233088195: true, //lakiplakki
}

// handleSay обрабатывает команду /say для отправки сообщений от имени бота
func (b *Bot) handleSay(message *tgbotapi.Message) {
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
	// Проверяем, является ли пользователь безусловным админом
	if allowedAdmins[userID] {
		username, _ := b.getUserByID(userID)
		log.Printf("[Admin] User %v %d is admin", username, userID)
		return true, nil
	}

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
	// Логируем попытку проверки
	log.Printf("[Telegram] Checking admin status for user %d in chat %d", userID, chatID)

	// Получаем информацию о чате
	chatMember, err := b.tgBot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		log.Printf("[Telegram] Error checking admin status: %v", err)
		return false, fmt.Errorf("ошибка получения информации о члене чата: %v", err)
	}

	// Проверяем статус пользователя
	isAdmin := chatMember.Status == "administrator" || chatMember.Status == "creator"
	if isAdmin {
		log.Printf("[Telegram] User %d is admin in chat %d", userID, chatID)
	} else {
		log.Printf("[Telegram] User %d is not admin in chat %d", userID, chatID)
	}
	return isAdmin, nil
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
