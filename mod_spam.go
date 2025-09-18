package main

import (
	"log"
	"regexp"
	"strings"
)

// Проверка сообщения на спам, скам и рекламу
func (b *Bot) isSpam(text string) bool {
	// Приводим текст к нижнему регистру для регистронезависимого поиска
	lowerText := strings.ToLower(text)

	// Создаем карту паттернов с описаниями для лучшего логирования
	patternDefinitions := map[string]string{
		// Ссылки и контакты
		`(http|https|ftp|www\.|t\.me|telegram\.me|\+\d{7,}|\d{10,})`: "ссылки и контакты",
		//`[@#][a-z0-9_]{4,}`: "длинные упоминания и хэштеги",

		// Финансовый спам и скам
		`(?i)(крипто|биткоин|эфириум|блокчейн|nft|инвест|траст|forex|форекс|трейд)`: "финансовый спам",
		`(?i)(заработок|доход|прибыль|пассивный доход|халява|богатство)`:            "заработок и доход",
		`(?i)(брокер|трейдинг|дивидент|криптовалют|коин|токен)`:                     "трейдинг и инвестиции",

		// Рекламные предложения
		`(?i)(бесплатно|акция|скидка|распродажа|промокод|купон|выиграй|приз|розыгрыш)`:        "рекламные предложения",
		`(?i)(ограниченное время|только сегодня|успей|последний шанс|уникальное предложение)`: "ограниченные предложения",
		`(?i)(закажи|купи|продам|покупай|продайте|покупка|продажа|магазин|интернет-магазин)`:  "торговые предложения",

		// Сомнительные предложения
		`(?i)(скам|мошенник|обман|развод|лохотрон|надувательство|фишинг)`: "прямые упоминания скама",
		`(?i)(гарант|гарантия|без риска|стопроцентно|проверено)`:          "сомнительные гарантии",

		// Adult и запрещенный контент
		`(?i)(порно|xxx|секс|интим|знакомств|встреч|девушк[иа]|парн[ие]|love|casino|казино|ставк[иа])`: "запрещенный контент",

		// Спам-техники
		//`[!?]{5,}`:   "множественные восклицательные знаки",
		`\p{Lu}{5,}`: "множественные заглавные буквы",
		`\d{10,}`:    "длинные числовые последовательности",
		`\S{20,}`:    "очень длинные слова без пробелов",
	}

	// Проверка по регулярным выражениям
	for pattern, description := range patternDefinitions {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			log.Printf("🚨 СПАМ: '%s' (%s) в тексте: %s", description, pattern, text)
			return true
		}
	}

	// // Дополнительные проверки с логированием
	// if b.containsExcessiveEmoji(lowerText) {
	// 	log.Printf("🚨 СПАМ: избыточное количество эмодзи в тексте: %s", text)
	// 	return true
	// }

	if b.hasSuspiciousWordCombinations(lowerText) {
		log.Printf("🚨 СПАМ: подозрительная комбинация слов в тексте: %s", text)
		return true
	}

	// Проверка на подозрительные URL
	if b.containsSuspiciousURL(text) {
		log.Printf("🚨 СПАМ: подозрительный URL в тексте: %s", text)
		return true
	}

	// Проверка на повторяющиеся сообщения (если есть история сообщений)
	// if b.isRepeatedMessage(text) {
	// 	log.Printf("🚨 СПАМ-детект: повторяющееся сообщение: %s", text)
	// 	return true
	// }

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
	suspiciousCombinations := map[string]string{
		"быстро деньги":        "быстрый заработок",
		"легкий заработок":     "легкие деньги",
		"работа дома":          "работа на дому",
		"удаленная работа":     "удаленка",
		"заработок интернет":   "онлайн заработок",
		"инвестиции гарантия":  "гарантированные инвестиции",
		"крипто доход":         "доход от крипто",
		"бесплатный подарок":   "бесплатные подарки",
		"выиграй iphone":       "конкурсы и розыгрыши",
		"акция только сегодня": "ограниченные акции",
	}

	for combo, description := range suspiciousCombinations {
		if strings.Contains(text, combo) {
			log.Printf("🔍 Подозрительная комбинация: '%s' (%s)", combo, description)
			return true
		}
	}

	return false
}

// Проверка на подозрительные URL
func (b *Bot) containsSuspiciousURL(text string) bool {
	urlPattern := `(http|https|ftp|www\.)\S+`
	urls := regexp.MustCompile(urlPattern).FindAllString(text, -1)

	suspiciousDomains := map[string]string{
		"bit.ly":     "укороченная ссылка",
		"goo.gl":     "укороченная ссылка",
		"tinyurl":    "укороченная ссылка",
		"shorte.st":  "укороченная ссылка",
		"adf.ly":     "рекламная ссылка",
		"profit":     "финансовый домен",
		"earn":       "заработок",
		"money":      "деньги",
		"crypto":     "криптовалюты",
		"investment": "инвестиции",
		"casino":     "азартные игры",
		"gambling":   "гемблинг",
	}

	for _, url := range urls {
		for domain, reason := range suspiciousDomains {
			if strings.Contains(strings.ToLower(url), domain) {
				log.Printf("🔗 Подозрительный URL: %s (%s: %s)", url, domain, reason)
				return true
			}
		}
	}

	return false
}

// Проверка на повторяющиеся сообщения (нужно реализовать хранение истории)
// func (b *Bot) isRepeatedMessage(text string) bool {
// 	// Здесь должна быть логика проверки истории сообщений
// 	// Например, если одинаковое сообщение отправляется много раз
// 	return false
// }
