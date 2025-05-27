package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) generateAiRequest(systemPrompt string, prompt string, message *tgbotapi.Message) (string, error) {
	// Отправляем индикатор печати
	if _, err := b.tgBot.Request(tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)); err != nil {
		log.Printf("Ошибка отправки индикатора печати: %v", err)
	}
	// Запускаем горутину для периодической отправки индикатора печати, канал stopTyping не забываем закрыть!!!
	stopTyping := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second) // Отправляем каждые 5 секунд
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Отправляем индикатор печати
				chatAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
				if _, err := b.tgBot.Request(chatAction); err != nil {
					log.Printf("Ошибка отправки индикатора печати: %v", err)
				}
			case <-stopTyping:
				return
			}
		}
	}()
	defer close(stopTyping)

	request := LocalLLMRequest{
		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
		MaxTokens:   16000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	log.Printf("Get AI %v data: %v", b.config.LocalLLMUrl, request)
	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
	}

	var response LocalLLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
	}

	log.Printf("Resp: %v", response)

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}

	summary := response.Choices[0].Message.Content
	if idx := strings.Index(summary, "--"); idx != -1 {
		summary = summary[:idx]
	}

	// // После получения ответа от AI сохраняем информацию о токенах
	if response.Usage.TotalTokens > 0 {
		record := BillingRecord{
			UserID:           message.From.ID,
			ChatID:           message.Chat.ID,
			Timestamp:        time.Now().Unix(),
			Model:            response.Model,
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
			Cost:             calculateCost(response.Model, response.Usage.TotalTokens),
		}

		if err := b.SaveBillingRecord(record); err != nil {
			log.Printf("Ошибка биллинга: %v", err)
		}
	}

	return strings.TrimSpace(summary), nil
}

// generateSummary создает краткую сводку с помощью локальной LLM
// func (b *Bot) generateSummary(messages string, chatID int64) (string, error) {
// 	// Отправляем индикатор печати
// 	if _, err := b.tgBot.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
// 		log.Printf("Ошибка отправки индикатора печати: %v", err)
// 	}
// 	// Запускаем горутину для периодической отправки индикатора печати, канал stopTyping не забываем закрыть!!!
// 	stopTyping := make(chan struct{})
// 	go func() {
// 		ticker := time.NewTicker(5 * time.Second) // Отправляем каждые 5 секунд
// 		defer ticker.Stop()
// 		for {
// 			select {
// 			case <-ticker.C:
// 				// Отправляем индикатор печати
// 				chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
// 				if _, err := b.tgBot.Request(chatAction); err != nil {
// 					log.Printf("Ошибка отправки индикатора печати: %v", err)
// 				}
// 			case <-stopTyping:
// 				return
// 			}
// 		}
// 	}()
// 	defer close(stopTyping)

// 	prompt := fmt.Sprintf(b.config.SummaryPrompt, messages)

// 	request := LocalLLMRequest{
// 		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
// 		Messages: []LocalLLMMessage{
// 			{
// 				Role:    "system",
// 				Content: b.config.SystemPrompt,
// 			},
// 			{
// 				Role:    "user",
// 				Content: prompt,
// 			},
// 		},
// 		Temperature: 0.6,
// 		MaxTokens:   16000,
// 	}

// 	jsonData, err := json.Marshal(request)
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
// 	}

// 	fmt.Println("Get AI request...")
// 	time.Sleep(15 * time.Second)

// 	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
// 	}

// 	var response LocalLLMResponse
// 	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
// 		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
// 	}

// 	if len(response.Choices) == 0 {
// 		return "", fmt.Errorf("пустой ответ от LLM")
// 	}
// 	fmt.Printf("Resp Tokens: %v \n", response.Usage.TotalTokens)

// 	summary := response.Choices[0].Message.Content
// 	if idx := strings.Index(summary, "--"); idx != -1 {
// 		summary = summary[:idx]
// 	}

// 	return strings.TrimSpace(summary), nil
// }

// generateSummary создает краткую сводку с помощью локальной LLM
// func (b *Bot) generateAnekdot(messages string, chatID int64) (string, error) {
// 	// Запускаем горутину для периодической отправки индикатора печати, канал stopTyping не забываем закрыть!!!
// 	stopTyping := make(chan struct{})
// 	defer close(stopTyping)
// 	go func() {
// 		ticker := time.NewTicker(5 * time.Second) // Отправляем каждые 5 секунд
// 		defer ticker.Stop()
// 		for {
// 			select {
// 			case <-ticker.C:
// 				if _, err := b.tgBot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
// 					log.Printf("Ошибка отправки индикатора печати: %v", err)
// 				}
// 			case <-stopTyping:
// 				return
// 			}
// 		}
// 	}()
// 	prompt := fmt.Sprintf(b.config.AnekdotPrompt, messages)

// 	request := LocalLLMRequest{
// 		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
// 		Messages: []LocalLLMMessage{
// 			// {
// 			// 	Role:    "system",
// 			// 	Content: b.config.SystemPrompt,
// 			// },
// 			{
// 				Role:    "user",
// 				Content: prompt,
// 			},
// 		},
// 		Temperature: 0.4,
// 		MaxTokens:   1000,
// 	}

// 	jsonData, err := json.Marshal(request)
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
// 	}

// 	fmt.Println("Get AI request...")
// 	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
// 	}

// 	var response LocalLLMResponse
// 	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
// 		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
// 	}

// 	if len(response.Choices) == 0 {
// 		return "", fmt.Errorf("пустой ответ от LLM")
// 	}
// 	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

// 	return response.Choices[0].Message.Content, nil
// }

// generateSummary создает краткую сводку с помощью локальной LLM
// func (b *Bot) generateTopic(messages string, chatID int64) (string, error) {
// 	prompt := fmt.Sprintf(b.config.TopicPrompt, messages)

// 	request := LocalLLMRequest{
// 		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
// 		Messages: []LocalLLMMessage{
// 			// {
// 			// 	Role:    "system",
// 			// 	Content: b.config.SystemPrompt,
// 			// },
// 			{
// 				Role:    "user",
// 				Content: prompt,
// 			},
// 		},
// 		Temperature: 0.4,
// 		MaxTokens:   1000,
// 	}

// 	jsonData, err := json.Marshal(request)
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
// 	}

// 	fmt.Println("Get AI request...")
// 	resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return "", fmt.Errorf("неверный статус код: %d", resp.StatusCode)
// 	}

// 	var response LocalLLMResponse
// 	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
// 		return "", fmt.Errorf("ошибка декодирования ответа: %v", err)
// 	}

// 	if len(response.Choices) == 0 {
// 		return "", fmt.Errorf("пустой ответ от LLM")
// 	}
// 	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

// 	return response.Choices[0].Message.Content, nil
// }
