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

// CommandHandler обрабатывает команды бота
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	if !b.isChatAllowed(message.Chat.ID) {
		b.sendMessage(message.Chat.ID, "Извините, я не работаю в этом чате.")
		return
	}

	if !b.canBotReadMessages(message.Chat.ID) {
		b.sendMessage(message.Chat.ID, "Мне нужны права администратора или участника в этой группе чтобы видеть сообщения.")
		return
	}

	switch message.Command() {
	case "start":
		b.handleStart(message)
	case "help":
		b.handleHelp(message)
	case "ping", "пинг":
		b.handlePing(message)
	case "summary", "саммари":
		b.handleSummary(message)
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
		b.handleMem(message)
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
func (b *Bot) handleSummary(message *tgbotapi.Message) {
	args := strings.Fields(message.CommandArguments())
	count := LIMIT_MSG

	if len(args) > 0 {
		if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
			count = num
			if count > LIMIT_MSG {
				count = LIMIT_MSG
				b.sendMessage(message.Chat.ID, fmt.Sprintf("Я помню только %d сообщений...", LIMIT_MSG))
			}
		}
	}

	b.handleSummaryRequest(message, count)
}

// handleClear обрабатывает команду /clear
func (b *Bot) handleClear(message *tgbotapi.Message) {
	b.DeleteUserContext(message.Chat.ID, message.From.ID)
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
	rand.Seed(time.Now().UnixNano())

	// Выбираем случайный ответ
	response := responses[rand.Intn(len(responses))]

	b.sendMessage(message.Chat.ID, response)
}

// handleMem обрабатывает команду /mem
func (b *Bot) handleMem(message *tgbotapi.Message) {
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

	// Отправляем индикатор печати
	if _, err := b.tgBot.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
		log.Printf("[GenerateImage] Ошибка отправки индикатора печати: %v", err)
	}

	// Запускаем горутину для периодической отправки индикатора печати
	stopTyping := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
				if _, err := b.tgBot.Request(chatAction); err != nil {
					log.Printf("[GenerateImage] Ошибка отправки индикатора печати: %v", err)
				}
			case <-stopTyping:
				return
			}
		}
	}()
	defer close(stopTyping)

	// Получаем описание из текста сообщения после команды
	description := strings.TrimSpace(message.CommandArguments())
	if description == "" {
		b.sendMessage(chatID, "Пожалуйста, укажите описание для изображения после команды /mem")
		return
	}

	// Генерируем изображение
	photo, err := b.GenerateImage(description, chatID)
	if err != nil {
		log.Printf("Ошибка генерации изображения: %v", err)
		b.sendMessage(chatID, "Не удалось сгенерировать изображение. Попробуйте снова.")
		return
	}

	// Отправляем изображение
	_, err = b.tgBot.Send(*photo)
	if err != nil {
		log.Printf("Ошибка отправки изображения: %v", err)
		b.sendMessage(chatID, "Не удалось отправить изображение. Попробуйте снова.")
	}
}
