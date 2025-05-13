package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// generateSummary создает краткую сводку с помощью локальной LLM
func (b *Bot) generateSummary(messages string) (string, error) {
	prompt := fmt.Sprintf(b.config.SummaryPrompt, messages)

	request := LocalLLMRequest{
		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			{
				Role:    "system",
				Content: b.config.SystemPrompt,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.6,
		MaxTokens:   16000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	fmt.Println("Get AI request...")
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

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}
	fmt.Printf("Resp Tokens: %v \n", response.Usage.TotalTokens)

	summary := response.Choices[0].Message.Content
	if idx := strings.Index(summary, "--"); idx != -1 {
		summary = summary[:idx]
	}

	return strings.TrimSpace(summary), nil
}

// generateSummary создает краткую сводку с помощью локальной LLM
func (b *Bot) generateAnekdot(messages string) (string, error) {
	prompt := fmt.Sprintf(b.config.AnekdotPrompt, messages)

	request := LocalLLMRequest{
		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			// {
			// 	Role:    "system",
			// 	Content: b.config.SystemPrompt,
			// },
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.4,
		MaxTokens:   1000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	fmt.Println("Get AI request...")
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

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}
	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

	return response.Choices[0].Message.Content, nil
}

// generateSummary создает краткую сводку с помощью локальной LLM
func (b *Bot) generateTopic(messages string) (string, error) {
	prompt := fmt.Sprintf(b.config.TopicPrompt, messages)

	request := LocalLLMRequest{
		Model: b.config.AiModelName, // Имя модели может быть любым для локальной LLM
		Messages: []LocalLLMMessage{
			// {
			// 	Role:    "system",
			// 	Content: b.config.SystemPrompt,
			// },
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.4,
		MaxTokens:   1000,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	fmt.Println("Get AI request...")
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

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от LLM")
	}
	fmt.Printf("Resp Tokens: %v", response.Usage.TotalTokens)

	return response.Choices[0].Message.Content, nil
}
