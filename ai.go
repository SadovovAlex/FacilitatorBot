package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) GenerateImage(description string, chatID int64) (*tgbotapi.PhotoConfig, error) {
	log.Printf("[GenerateImage] Начало генерации изображения для chatID: %d", chatID)
	log.Printf("[GenerateImage] Описание: %s", description)

	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Подготовка URL для запроса
	url := b.config.AIImageURL + url.QueryEscape(description)
	log.Printf("[GenerateImage] URL запроса: %s", url)

	// Выполнение HTTP GET запроса с таймаутом
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("[GenerateImage] Ошибка создания HTTP запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %v", err)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GenerateImage] Ошибка выполнения запроса к API: %v", err)
		return nil, fmt.Errorf("ошибка при выполнении запроса к API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[GenerateImage] API вернул ошибку: %s", resp.Status)
		return nil, fmt.Errorf("API вернул ошибку: %s", resp.Status)
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[GenerateImage] Ошибка чтения ответа: %v", err)
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Создание сообщения с изображением
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "image.jpg",
		Bytes: body,
	})
	photo.Caption = description

	elapsed := time.Since(start)
	log.Printf("[GenerateImage] Успешно сгенерировано изображение для chatID: %d. Время: %v", chatID, elapsed)

	return &photo, nil
}

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
