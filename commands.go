package main

import (
	"fmt"
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
