package main

import (
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// initDB инициализирует базу данных
func (b *Bot) initDB() error {
	// Создаем таблицу чатов
	_, err := b.db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			id INTEGER PRIMARY KEY,
			title TEXT,
			type TEXT,
			username TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы чатов: %v", err)
	}

	// Создаем таблицу пользователей
	_, err = b.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы пользователей: %v", err)
	}

	// Создаем таблицу сообщений
	_, err = b.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY,
			chat_id INTEGER,
			user_id INTEGER,
			text TEXT,
			timestamp INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(chat_id) REFERENCES chats(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы сообщений: %v", err)
	}

	return nil
}

// saveChat сохраняет информацию о чате в БД
func (b *Bot) saveChat(chat *tgbotapi.Chat) error {
	if chat == nil {
		return nil
	}

	_, err := b.db.Exec(`
		INSERT OR IGNORE INTO chats (id, title, type, username) 
		VALUES (?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Type, chat.UserName)

	return err
}

// saveUser сохраняет информацию о пользователе в БД, 136817688  это сообщения от имени канала
func (b *Bot) saveUser(user *tgbotapi.User) error {
	if user == nil {
		return nil
	}

	firstName := user.FirstName
	if user.ID == 136817688 {
		firstName = "Админ-Канала"
	}

	result, err := b.db.Exec(`
		INSERT OR IGNORE INTO users (id, username, first_name, last_name) 
		VALUES (?, ?, ?, ?)`,
		user.ID, user.UserName, firstName, user.LastName)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected > 0 {
		fmt.Printf("Saved user: ID=%d, Username=%s, FirstName=%s, LastName=%s", user.ID, user.UserName, user.FirstName, user.LastName)
	}

	return nil
}

// saveMessage сохраняет сообщение в БД
func (b *Bot) saveMessage(chatID, userID int64, text string, timestamp int64) error {
	_, err := b.db.Exec(`
		INSERT INTO messages (chat_id, user_id, text, timestamp) 
		VALUES (?, ?, ?, ?)`,
		chatID, userID, text, timestamp)

	return err
}

// getRecentMessages получает сообщения за последние 6 часов
func (b *Bot) getRecentMessages(chatID int64, limit int) ([]DBMessage, error) {
	sixHoursAgo := time.Now().Add(CHECK_HOURS * time.Hour).Unix()

	// Если лимит не задан, устанавливаем его в 0, чтобы получить все сообщения
	if limit == 0 {
		limit = -1
	}

	query := `
		SELECT m.id, m.chat_id, m.user_id, u.username, u.first_name, u.last_name, m.text, m.timestamp, 
		       c.title as chat_title
		FROM messages m
		LEFT JOIN users u ON m.user_id = u.id
		LEFT JOIN chats c ON m.chat_id = c.id
		WHERE m.timestamp >= ? 
		AND m.chat_id = ?
		ORDER BY m.timestamp desc
		LIMIT ?
	`

	rows, err := b.db.Query(query, sixHoursAgo, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса сообщений: %v", err)
	}
	defer rows.Close()

	var messages []DBMessage
	for rows.Next() {
		var msg DBMessage
		err := rows.Scan(
			&msg.ID,
			&msg.ChatID,
			&msg.UserID,
			&msg.Username,
			&msg.UserFirstName,
			&msg.UserLastName,
			&msg.Text,
			&msg.Timestamp,
			&msg.ChatTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения сообщения: %v", err)
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка обработки результатов: %v", err)
	}

	//fmt.Printf("mmmm: %--v", messages)

	return messages, nil
}
