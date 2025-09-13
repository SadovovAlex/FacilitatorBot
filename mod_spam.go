package main

import (
	"regexp"
	"strings"
)

// Проверка сообщения на спам, скам и рекламу
func (b *Bot) isSpam(text string) bool {
	// Приводим текст к нижнему регистру для регистронезависимого поиска
	lowerText := strings.ToLower(text)

	// Регулярные выражения для различных типов спама
	patterns := []string{
		// Ссылки и контакты
		`(http|https|ftp|www\.|t\.me|telegram\.me|@[\w_]{5,}|\+\d{7,}|\d{10,})`,
		`[@#][a-z0-9_]{4,}`, // Длинные упоминания и хэштеги

		// Финансовый спам и скам
		`(?i)(крипто|биткоин|эфириум|блокчейн|nft|инвест|вклад|депозит|траст|forex|форекс|fx|трейд)`,
		`(?i)(заработок|доход|прибыль|пассивный доход|халява|деньги|богатство|миллион)`,
		`(?i)(брокер|трейдинг|акции|дивидент|криптовалют|коин|токен)`,

		// Рекламные предложения
		`(?i)(бесплатно|акция|скидка|распродажа|промокод|купон|выиграй|приз|розыгрыш)`,
		`(?i)(ограниченное время|только сегодня|успей|последний шанс|уникальное предложение)`,
		`(?i)(закажи|купи|продам|покупай|продайте|покупка|продажа|магазин|интернет-магазин)`,

		// Сомнительные предложения
		`(?i)(скам|мошенник|обман|развод|лохотрон|надувательство|фишинг)`,
		`(?i)(гарант|гарантия|без риска|100%|стопроцентно|проверено)`,

		// Adult и запрещенный контент
		`(?i)(порно|xxx|секс|интим|знакомств|встреч|девушк[иа]|парн[ие]|love|casino|казино|ставк[иа])`,

		// Спам-техники
		`[!?]{3,}`,   // Множественные восклицательные знаки
		`\p{Lu}{5,}`, // Множественные заглавные буквы (с учетом Unicode)
		`\d{10,}`,    // Длинные числовые последовательности
		`\S{20,}`,    // Очень длинные слова без пробелов
	}

	// Проверка по регулярным выражениям
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}

	// Дополнительные проверки
	if b.containsExcessiveEmoji(lowerText) {
		return true
	}

	if b.hasSuspiciousWordCombinations(lowerText) {
		return true
	}

	// Проверка на повторяющиеся сообщения (если есть история сообщений)
	if b.isRepeatedMessage(text) {
		return true
	}

	return false
}

// Проверка на избыточное количество эмодзи
func (b *Bot) containsExcessiveEmoji(text string) bool {
	emojiPattern := `[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F700}-\x{1F77F}\x{1F780}-\x{1F7FF}\x{1F800}-\x{1F8FF}\x{1F900}-\x{1F9FF}\x{1FA00}-\x{1FA6F}\x{1FA70}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}]`

	emojiCount := len(regexp.MustCompile(emojiPattern).FindAllString(text, -1))
	textLength := len([]rune(text))

	// Если более 30% текста - эмодзи, считаем спамом
	if textLength > 0 && float64(emojiCount)/float64(textLength) > 0.3 {
		return true
	}

	return false
}

// Проверка подозрительных комбинаций слов
func (b *Bot) hasSuspiciousWordCombinations(text string) bool {
	suspiciousCombinations := []string{
		"быстро деньги",
		"легкий заработок",
		"работа дома",
		"удаленная работа",
		"заработок интернет",
		"инвестиции гарантия",
		"крипто доход",
		"бесплатный подарок",
		"выиграй iPhone",
		"акция только сегодня",
	}

	for _, combo := range suspiciousCombinations {
		if strings.Contains(text, combo) {
			return true
		}
	}

	return false
}

// Проверка на повторяющиеся сообщения (нужно реализовать хранение истории)
func (b *Bot) isRepeatedMessage(text string) bool {
	// Здесь должна быть логика проверки истории сообщений
	// Например, если одинаковое сообщение отправляется много раз
	return false
}

// Дополнительная функция для проверки URL (если нужно отдельно)
func (b *Bot) containsSuspiciousURL(text string) bool {
	urlPattern := `(http|https|ftp|www\.)\S+`
	urls := regexp.MustCompile(urlPattern).FindAllString(text, -1)

	suspiciousDomains := []string{
		"bit.ly", "goo.gl", "tinyurl", "shorte.st", "adf.ly",
		"profit", "earn", "money", "crypto", "investment",
	}

	for _, url := range urls {
		for _, domain := range suspiciousDomains {
			if strings.Contains(url, domain) {
				return true
			}
		}
	}

	return false
}
