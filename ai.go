package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"facilitatorbot/db"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) GenerateImage(description string, chatID int64, enableDescription bool) (*tgbotapi.PhotoConfig, error) {
	log.Printf("[GenerateImage] Генерация img для chatID: %d Описание: %v", chatID, description)

	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Подготовка URL для запроса
	url := fmt.Sprintf("%s%s", b.config.AIImageURL, url.QueryEscape(description))
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
		elapsed := time.Since(start)
		log.Printf("[GenerateImage] Ошибка выполнения запроса к API. Время: %v, Ошибка: %v", elapsed, err)
		return nil, fmt.Errorf("ошибка при выполнении запроса к API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[GenerateImage] API вернул ошибку: %s", resp.Status)
		return nil, fmt.Errorf("API вернул ошибку: %s", resp.Status)
	}

	// Чтение ответа
	// Логируем статус ответа
	log.Printf("[GenerateImage] от API. Статус: %s", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		elapsed := time.Since(start)
		log.Printf("[GenerateImage] Ошибка чтения ответа. Время: %v, Ошибка: %v", elapsed, err)
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Логируем успешный ответ
	elapsed := time.Since(start)
	log.Printf("[GenerateImage] Успешно получен ответ. Время: %v", elapsed)

	// Обработка изображения
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		log.Printf("[GenerateImage] Ошибка декодирования изображения: %v", err)
		return nil, fmt.Errorf("ошибка декодирования изображения: %v", err)
	}

	// Получаем размеры изображения
	bounds := img.Bounds()
	width := bounds.Max.X
	height := bounds.Max.Y

	// Обрезаем нижнюю часть на 150 пикселей
	if height > 60 {
		height -= 60
		img = img.(interface {
			SubImage(r image.Rectangle) image.Image
		}).SubImage(image.Rect(0, 0, width, height))
	}

	// Создаем буфер для нового изображения
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		log.Printf("[GenerateImage] Ошибка кодирования изображения: %v", err)
		return nil, fmt.Errorf("ошибка кодирования изображения: %v", err)
	}

	// Создание сообщения с изображением
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "aiimage.jpg",
		Bytes: buf.Bytes(),
	})

	if enableDescription {
		// Обрезаем caption до максимальной длины Telegram API (128 символа)
		if len(description) > 1024 {
			description = description[:1024] + "..."
		}

		// Проверяем и исправляем UTF-8 кодировку
		if !utf8.ValidString(description) {
			// Если строка не в UTF-8, преобразуем её
			utf8Description := string([]rune(description))
			description = utf8Description
		}

		photo.Caption = description
	}

	elapsed = time.Since(start)
	log.Printf("[GenerateImage] Cгенерировано img для chatID: %d. Время: %v", chatID, elapsed)

	return &photo, nil
}

func (b *Bot) generateAiRequest(systemPrompt string, prompt string, message *tgbotapi.Message) (string, error) {
	// Логируем параметры запроса
	log.Printf("[generateAiRequest] Начало запроса к AI. ChatID: %d, Model: %s", message.Chat.ID, b.config.AiModelName)
	log.Printf("[generateAiRequest] System prompt: %s", systemPrompt)
	log.Printf("[generateAiRequest] User prompt[%d]: %v", len(prompt), b.truncateText(prompt, 256))

	request := LocalLLMRequest{
		Model: b.config.AiModelName,
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

	// Retry logic with exponential backoff
	const maxRetries = 3
	baseDelay := 60000 * time.Millisecond
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Логируем отправку запроса
		log.Printf("[generateAiRequest] Попытка %d/%d. Отправка запроса к %s", attempt+1, maxRetries, b.config.LocalLLMUrl)

		resp, err := b.httpClient.Post(b.config.LocalLLMUrl, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("[generateAiRequest] Ошибка HTTP запроса (попытка %d): %v", attempt+1, err)
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("ошибка HTTP запроса после %d попыток: %v", maxRetries, err)
			}
			time.Sleep(baseDelay * time.Duration(2<<uint(attempt)))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("[generateAiRequest] Неверный статус код (попытка %d): %d", attempt+1, resp.StatusCode)
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("неверный статус код после %d попыток: %d", maxRetries, resp.StatusCode)
			}
			time.Sleep(baseDelay * time.Duration(2<<uint(attempt)))
			continue
		}

		var response LocalLLMResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			log.Printf("[generateAiRequest] Ошибка декодирования ответа (попытка %d): %v", attempt+1, err)
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("ошибка декодирования ответа после %d попыток: %v", maxRetries, err)
			}
			time.Sleep(baseDelay * time.Duration(2<<uint(attempt)))
			continue
		}

		if len(response.Choices) == 0 {
			log.Printf("[generateAiRequest] Пустой ответ от LLM (попытка %d)", attempt+1)
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("пустой ответ от LLM после %d попыток", maxRetries)
			}
			time.Sleep(baseDelay * time.Duration(2<<uint(attempt)))
			continue
		}

		// Успешный ответ
		log.Printf("[generateAiRequest] Успешный ответ получен (попытка %d)", attempt+1)
		summary := response.Choices[0].Message.Content
		if idx := strings.Index(summary, "--"); idx != -1 {
			summary = summary[:idx]
		}

		// // После получения ответа от AI сохраняем информацию о токенах
		if response.Usage.TotalTokens > 0 {
			record := db.BillingRecord{
				UserID:           message.From.ID,
				ChatID:           message.Chat.ID,
				Timestamp:        time.Now().Unix(),
				Model:            response.Model,
				PromptTokens:     response.Usage.PromptTokens,
				CompletionTokens: response.Usage.CompletionTokens,
				TotalTokens:      response.Usage.TotalTokens,
				Cost:             calculateCost(response.Model, response.Usage.TotalTokens),
			}

			if err := b.db.SaveBillingRecord(record); err != nil {
				log.Printf("Ошибка биллинга: %v", err)
			}
		}

		return strings.TrimSpace(summary), nil
	}

	return "", fmt.Errorf("все %d попытки завершились неудачей", maxRetries)
}
