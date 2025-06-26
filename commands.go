package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
		b.handleAIStats(message)
	case "anekdot", "анекдот":
		b.handleAnekdot(message)
	case "tema", "topic":
		b.handleTopic(message)
	case "clear", "забудь":
		b.handleClear(message)
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
	b.sendMessage(message.Chat.ID, "pong")

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

// handleStats обрабатывает команду /stats
func (b *Bot) handleStats(message *tgbotapi.Message) {
	b.handleStatsRequest(message)
}

// handleAIStats обрабатывает команду /aistats (только для администраторов)
func (b *Bot) handleAIStats(message *tgbotapi.Message) {
	if allowedAdmins[message.From.ID] {
		b.handleGetTopAIUsers(message)
	}
}

// handleAnekdot обрабатывает команду /anekdot
func (b *Bot) handleAnekdot(message *tgbotapi.Message) {
	b.handleAnekdotRequest(message)
}

// handleTopic обрабатывает команду /tema
func (b *Bot) handleTopic(message *tgbotapi.Message) {
	b.handleTopicRequest(message)
}

// handleClear обрабатывает команду /clear
func (b *Bot) handleClear(message *tgbotapi.Message) {
	b.DeleteUserContext(message.Chat.ID, message.From.ID)
}

// handleUnknownCommand обрабатывает неизвестные команды
func (b *Bot) handleUnknownCommand(message *tgbotapi.Message) {
	b.sendMessage(message.Chat.ID, "Неизвестная команда. Используйте /help для списка команд.")
}
