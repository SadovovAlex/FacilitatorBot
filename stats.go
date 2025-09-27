package main

// // handleGetTopAIUsers возвращает топ пользователей по использованию токенов в читаемом формате
// func (b *Bot) handleGetTopAIUsers(message *tgbotapi.Message) {
// 	// Проверяем права пользователя (только админы могут запрашивать статистику)
// 	// if !b.isUserAdmin(message.Chat.ID, message.From.ID) {
// 	// 	b.sendMessage(message.Chat.ID, "🚫 У вас нет прав.")
// 	// 	return
// 	// }

// 	// Получаем топ 10 пользователей за последние 30 дней
// 	topUsers, err := b.GetTopUsersByTokenUsage(10, 30)
// 	if err != nil {
// 		log.Printf("Ошибка получения топ пользователей: %v", err)
// 		b.sendMessage(message.Chat.ID, "⚠️ Произошла ошибка при получении статистики.")
// 		return
// 	}

// 	if len(topUsers) == 0 {
// 		b.sendMessage(message.Chat.ID, "📊 Нет данных об использовании AI за последние 30 дней.")
// 		return
// 	}

// 	// Получаем общую статистику по чату
// 	chatStats, err := b.GetChatTokenUsage(message.Chat.ID, 30)
// 	if err != nil {
// 		log.Printf("Ошибка получения статистики чата: %v", err)
// 	}

// 	// Формируем красивое сообщение
// 	var reply strings.Builder
// 	reply.WriteString("📊 <b>Топ пользователей по использованию AI</b>\n")
// 	reply.WriteString("⏱ Период: последние 30 дней\n\n")

// 	// Добавляем общую статистику по чату
// 	if chatStats.TotalTokens > 0 {
// 		reply.WriteString("💬 <b>Общее по чату:</b>\n")
// 		reply.WriteString(fmt.Sprintf("🪙 Токены: %d (запросы: %d, ответы: %d)\n",
// 			chatStats.TotalTokens, chatStats.PromptTokens, chatStats.CompletionTokens))
// 		reply.WriteString(fmt.Sprintf("💵 Примерная стоимость: $%.2f\n\n", chatStats.Cost))
// 	}

// 	reply.WriteString("🏆 <b>Топ пользователей:</b>\n")

// 	for i, user := range topUsers {
// 		// Получаем информацию о пользователе
// 		username, err := b.getUserByID(user.UserID)
// 		if err != nil || username == nil {
// 			continue
// 		}

// 		// Форматируем строку для каждого пользователя
// 		reply.WriteString(fmt.Sprintf("%d. %s:\n", i+1, username))
// 		reply.WriteString(fmt.Sprintf("   🪙 Токены: %d\n", user.TotalTokens))
// 		reply.WriteString(fmt.Sprintf("   💵 Примерная стоимость: $%.2f\n\n", user.Cost))
// 	}

// 	// Добавляем подсказку
// 	//reply.WriteString("\nℹ️ Для получения детальной статистики используйте /aitokens @username")

// 	// Отправляем сообщение
// 	msg := tgbotapi.NewMessage(message.Chat.ID, reply.String())
// 	msg.ParseMode = "HTML"
// 	if _, err := b.tgBot.Send(msg); err != nil {
// 		log.Printf("Ошибка отправки сообщения: %v", err)
// 	}
// }

// // handleStatsRequest показывает статистику по сообщениям и благодарностям из БД
// func (b *Bot) handleStatsRequest(message *tgbotapi.Message) {
// 	chatID := message.Chat.ID

// 	// Формируем сообщение со статистикой
// 	var statsMsg strings.Builder
// 	fmt.Fprintf(&statsMsg, "📊 Статистика чата:\n\n")

// 	// 1. Общая статистика по сообщениям
// 	var totalMessages int
// 	err := b.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&totalMessages)
// 	if err == nil {
// 		fmt.Fprintf(&statsMsg, "📨 Всего сообщений: %d\n", totalMessages)
// 	}

// 	// 2. Статистика по благодарностям
// 	var totalThanks int
// 	err = b.db.QueryRow("SELECT COUNT(*) FROM mod_thanks WHERE chat_id = ?", chatID).Scan(&totalThanks)
// 	if err == nil {
// 		fmt.Fprintf(&statsMsg, "🙏 Всего благодарностей: %d\n\n", totalThanks)
// 	}

// 	// 3. Топ получателей благодарностей
// 	fmt.Fprintf(&statsMsg, "🏆 Топ-5 самых благодарных пользователей:\n")
// 	rows, err := b.db.Query(`
//         SELECT u.username, COUNT(*) as thanks_count
//         FROM mod_thanks t
//         JOIN users u ON t.from_user_id = u.id
//         WHERE t.chat_id = ?
//         GROUP BY t.from_user_id
//         ORDER BY thanks_count DESC
//         LIMIT 5
//     `, chatID)
// 	if err == nil {
// 		defer rows.Close()
// 		rank := 1
// 		for rows.Next() {
// 			var username string
// 			var count int
// 			if err := rows.Scan(&username, &count); err != nil {
// 				continue
// 			}
// 			if username == "" {
// 				username = "Без username"
// 			}
// 			fmt.Fprintf(&statsMsg, "%d. %s - %d раз\n", rank, username, count)
// 			rank++
// 		}
// 	}

// 	// 4. Топ получателей благодарностей
// 	fmt.Fprintf(&statsMsg, "\n💖 Топ-5 самых ценных участников:\n")
// 	rows, err = b.db.Query(`
//         SELECT u.username, COUNT(*) as thanks_received
//         FROM mod_thanks t
//         JOIN users u ON t.to_user_id = u.id
//         WHERE t.chat_id = ? AND t.to_user_id != 0
//         GROUP BY t.to_user_id
//         ORDER BY thanks_received DESC
//         LIMIT 5
//     `, chatID)
// 	if err == nil {
// 		defer rows.Close()
// 		rank := 1
// 		for rows.Next() {
// 			var username string
// 			var count int
// 			if err := rows.Scan(&username, &count); err != nil {
// 				continue
// 			}
// 			if username == "" {
// 				username = "Без username"
// 			}
// 			fmt.Fprintf(&statsMsg, "%d. %s - %d благодарностей\n", rank, username, count)
// 			rank++
// 		}
// 	}

// 	// 5. Последние благодарности
// 	fmt.Fprintf(&statsMsg, "\n🆕 Последние благодарности:\n")
// 	rows, err = b.db.Query(`
//         SELECT u1.username, u2.username, t.text
//         FROM mod_thanks t
//         LEFT JOIN users u1 ON t.from_user_id = u1.id
//         LEFT JOIN users u2 ON t.to_user_id = u2.id
//         WHERE t.chat_id = ?
//         ORDER BY t.timestamp DESC
//         LIMIT 3
//     `, chatID)
// 	if err == nil {
// 		defer rows.Close()
// 		for rows.Next() {
// 			var fromUser, toUser, text string
// 			if err := rows.Scan(&fromUser, &toUser, &text); err != nil {
// 				continue
// 			}
// 			if fromUser == "" {
// 				fromUser = "Аноним"
// 			}
// 			if toUser == "" {
// 				toUser = "всех"
// 			}
// 			fmt.Fprintf(&statsMsg, "👉 %s → %s: %s\n", fromUser, toUser, truncateText(text, 20))
// 		}
// 	}

// 	// 6. Статистика за последние сутки
// 	dayAgo := time.Now().Add(-24 * time.Hour).Unix()
// 	var lastDayThanks int
// 	err = b.db.QueryRow(`
//         SELECT COUNT(*)
//         FROM mod_thanks
//         WHERE chat_id = ? AND timestamp >= ?
//     `, chatID, dayAgo).Scan(&lastDayThanks)
// 	if err == nil {
// 		fmt.Fprintf(&statsMsg, "\n🕒 Благодарностей за сутки: %d", lastDayThanks)
// 	}

// 	b.sendMessage(chatID, statsMsg.String())
// }
