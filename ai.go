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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) GenerateImage(description string, chatID int64, enableDescription bool) (*tgbotapi.PhotoConfig, error) {
	log.Printf("[GenerateImage] Генерация img для chatID: %d", chatID)
	log.Printf("[GenerateImage] Описание: %vs", b.truncateText(description, 512))

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

	// Логируем отправку запроса
	log.Printf("[generateAiRequest] Отправка запроса к %s", b.config.LocalLLMUrl)

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
