package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const CHECK_HOURS = -24

type DB struct {
	db                   *sql.DB
	HistoryDays          int
	ContextRetentionDays int
}

func NewDB(path string, historyDays, contextRetentionDays int) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия базы данных: %v", err)
	}
	return &DB{
		db:                   sqlDB,
		HistoryDays:          historyDays,
		ContextRetentionDays: contextRetentionDays,
	}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Init() error {
	return RunMigrations(d.db)
}

type ContextMessage struct {
	Role      string
	Content   string
	Timestamp int64
}

type DBMessage struct {
	ID            int
	ChatID        int64
	UserID        int64
	Username      string
	UserFirstName string
	UserLastName  string
	Text          string
	Timestamp     int64
	ChatTitle     string
}

type BillingRecord struct {
	UserID           int64
	ChatID           int64
	Timestamp        int64
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64
}

func (d *DB) LogIncident(chatID int64, userID int64, text string, timestamp int64, reason string) error {
	_, err := d.db.Exec(
		`INSERT INTO mod_spam_incidents
		(chat_id, user_id, message_text, reason, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		chatID, userID, text, time.Unix(timestamp, 0).Format(time.RFC3339), reason,
	)
	return err
}

func (d *DB) IsNewUserInChat(chatID, userID int64) (bool, error) {
	query := `
		SELECT COUNT(*) FROM messages 
		WHERE chat_id = ? AND user_id = ?
	`
	var messageCount int
	err := d.db.QueryRow(query, chatID, userID).Scan(&messageCount)
	if err != nil {
		return false, err
	}

	return messageCount < 5, nil
}

// SaveChat сохраняет информацию о чате в БД
func (d *DB) SaveChat(chat *tgbotapi.Chat) error {
	if chat == nil {
		return nil
	}

	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO chats (id, title, type, username) 
		VALUES (?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Type, chat.UserName)

	return err
}

// SaveUser сохраняет информацию о пользователе в БД, 136817688  это сообщения от имени канала
func (d *DB) SaveUser(message *tgbotapi.Message) error {
	if message.From == nil {
		return nil
	}

	firstName := message.From.UserName
	if message.From.ID == 136817688 {
		firstName = "Админ-Канала"
	}

	result, err := d.db.Exec(`
		INSERT OR IGNORE INTO users (id, username, first_name, last_name) 
		VALUES (?, ?, ?, ?)`,
		message.From.ID, message.From.UserName, firstName, message.From.LastName)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected > 0 {
		fmt.Printf("Saved user: ID=%d, Username=%s, FirstName=%s, LastName=%s", message.From.ID, message.From.UserName, message.From.FirstName, message.From.LastName)
	}

	return nil
}

// SaveMessage сохраняет сообщение в БД
func (d *DB) SaveMessage(chatID, userID int64, text string, timestamp int64) error {
	_, err := d.db.Exec(`
		INSERT INTO messages (chat_id, user_id, text, timestamp) 
		VALUES (?, ?, ?, ?)`,
		chatID, userID, text, timestamp)

	return err
}

// SaveThanks сохраняет благодарность с информацией о получателе
func (d *DB) SaveThanks(chatID, fromUserID, toUserID int64, text string, timestamp int64, messageID int) error {
	_, err := d.db.Exec(`
        INSERT INTO mod_thanks (chat_id, from_user_id, to_user_id, text, timestamp, message_id) 
        VALUES (?, ?, ?, ?, ?, ?)`,
		chatID, fromUserID, toUserID, text, timestamp, messageID)

	return err
}

// GetUserByUsername получает пользователя по username из БД
func (d *DB) GetUserByUsername(username string) (*tgbotapi.User, error) {
	var user tgbotapi.User
	err := d.db.QueryRow(`
        SELECT id, first_name, last_name, username 
        FROM users 
        WHERE username = ?`, username).Scan(
		&user.ID, &user.FirstName, &user.LastName, &user.UserName)

	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetRecentMessages получает сообщения за последние [limit] часов
func (d *DB) GetRecentMessages(chatID int64, limit int) ([]DBMessage, error) {
	hoursAgo := time.Now().Add(CHECK_HOURS * time.Hour).Unix()

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

	rows, err := d.db.Query(query, hoursAgo, chatID, limit)
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

	return messages, nil
}

// SaveContext сохраняет контекст сообщения в БД
func (d *DB) SaveContext(chatID, userID int64, role, content string, timestamp int64) error {
	_, err := d.db.Exec(`
		INSERT INTO chat_context (chat_id, user_id, role, content, timestamp) 
		VALUES (?, ?, ?, ?, ?)`,
		chatID, userID, role, content, timestamp)
	return err
}

// GetConversationContext получает контекст общения для указанного чата и пользователя
// limitMessages - максимальное количество сообщений для возврата (0 - без ограничения)
// timeLimitHours - максимальный возраст сообщений в часах (0 - без ограничения)
func (d *DB) GetConversationContext(chatID, userID int64, limitMessages int, timeLimitHours int) ([]ContextMessage, error) {
	var query string
	var args []interface{}

	query = `
		SELECT role, content, timestamp 
		FROM chat_context 
		WHERE chat_id = ? AND user_id = ?
	`

	args = append(args, chatID, userID)

	if timeLimitHours > 0 {
		timeLimit := time.Now().Add(-time.Duration(timeLimitHours) * time.Hour).Unix()
		query += " AND timestamp >= ?"
		args = append(args, timeLimit)
	}

	query += " ORDER BY timestamp DESC"

	if limitMessages > 0 {
		query += " LIMIT ?"
		args = append(args, limitMessages)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса контекста: %v", err)
	}
	defer rows.Close()

	var context []ContextMessage
	for rows.Next() {
		var msg ContextMessage
		err := rows.Scan(&msg.Role, &msg.Content, &msg.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения контекста: %v", err)
		}
		context = append(context, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка обработки результатов: %v", err)
	}

	// Переворачиваем порядок, чтобы хронология была правильной
	for i, j := 0, len(context)-1; i < j; i, j = i+1, j-1 {
		context[i], context[j] = context[j], context[i]
	}

	return context, nil
}

// DeleteUserContext удаляет контекст общения для указанного пользователя в чате
func (d *DB) DeleteUserContext(chatID, userID int64) error {
	if d.db == nil {
		return fmt.Errorf("база данных не инициализирована")
	}

	_, err := d.db.Exec(`
        DELETE FROM chat_context 
        WHERE chat_id = ? AND user_id = ?`,
		chatID, userID)

	if err != nil {
		return fmt.Errorf("ошибка удаления контекста: %v", err)
	}

	log.Printf("Контекст удален для user_id %d в chat_id %d", userID, chatID)
	return nil
}

// SaveBillingRecord сохраняет информацию об использовании токенов
func (d *DB) SaveBillingRecord(record BillingRecord) error {
	_, err := d.db.Exec(`
        INSERT INTO ai_billing 
        (user_id, chat_id, timestamp, model, prompt_tokens, completion_tokens, total_tokens, cost)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.UserID,
		record.ChatID,
		record.Timestamp,
		record.Model,
		record.PromptTokens,
		record.CompletionTokens,
		record.TotalTokens,
		record.Cost)

	if err != nil {
		return fmt.Errorf("ошибка сохранения записи биллинга: %v", err)
	}
	return nil
}

// GetChatTokenUsage возвращает статистику использования токенов в чате
func (d *DB) GetChatTokenUsage(chatID int64, days int) (BillingRecord, error) {
	var result BillingRecord
	var threshold int64

	if days > 0 {
		threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}

	query := `
        SELECT 
            SUM(prompt_tokens) as prompt_tokens,
            SUM(completion_tokens) as completion_tokens,
            SUM(total_tokens) as total_tokens,
            SUM(cost) as cost
        FROM ai_billing
        WHERE chat_id = ?`

	args := []interface{}{chatID}

	if days > 0 {
		query += " AND timestamp >= ?"
		args = append(args, threshold)
	}

	err := d.db.QueryRow(query, args...).Scan(
		&result.PromptTokens,
		&result.CompletionTokens,
		&result.TotalTokens,
		&result.Cost)

	if err != nil {
		return BillingRecord{}, fmt.Errorf("ошибка получения статистики токенов: %v", err)
	}

	result.ChatID = chatID
	return result, nil
}

func (d *DB) GetTopUsersByTokenUsage(limit int, days int) ([]struct {
	UserID      int64
	TotalTokens int
	Cost        float64
}, error) {
	var threshold int64
	if days > 0 {
		threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}

	query := `
        SELECT 
            user_id,
            SUM(total_tokens) as total_tokens,
            SUM(cost) as cost
        FROM ai_billing`

	args := []interface{}{}

	if days > 0 {
		query += " WHERE timestamp >= ?"
		args = append(args, threshold)
	}

	query += " GROUP BY user_id ORDER BY total_tokens DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса топ пользователей: %v", err)
	}
	defer rows.Close()

	var result []struct {
		UserID      int64
		TotalTokens int
		Cost        float64
	}

	for rows.Next() {
		var item struct {
			UserID      int64
			TotalTokens int
			Cost        float64
		}
		if err := rows.Scan(&item.UserID, &item.TotalTokens, &item.Cost); err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

// CleanupOldContext удаляет старый контекст общения
func (d *DB) CleanupOldContext() {
	for {
		time.Sleep(12 * time.Hour) // Проверяем каждые 12 часов
		// Удаляем контекст старше 7 дней (или другого значения из конфига)
		threshold := time.Now().Add(-time.Duration(d.ContextRetentionDays) * 24 * time.Hour).Unix()
		_, err := d.db.Exec("DELETE FROM chat_context WHERE timestamp < ?", threshold)
		if err != nil {
			log.Printf("Ошибка очистки старого контекста: %v", err)
		}
	}
}

// DeleteOldMessages удаляет сообщения старше указанного количества дней
func (d *DB) DeleteOldMessages() error {
	for {
		time.Sleep(12 * time.Hour) // Проверяем каждые 12 часов
		threshold := time.Now().Add(-time.Duration(d.HistoryDays) * 24 * time.Hour).Unix()
		_, err := d.db.Exec(`
			DELETE FROM messages 
			WHERE timestamp < ?`,
			threshold)

		if err != nil {
			log.Printf("ошибка удаления старых сообщений: %v", err)
		}

		log.Printf("Удалены сообщения старше %d дней", d.HistoryDays)
	}

}

// GetUserAIInfo получает информацию о настройках AI пользователя
func (d *DB) GetUserAIInfo(userID int64) (string, error) {
	var aiInfo string
	err := d.db.QueryRow(`
        SELECT ai_user_info FROM users WHERE id = ?`, userID).Scan(&aiInfo)
	if err != nil {
		return "", fmt.Errorf("ошибка получения информации о пользователе: %v", err)
	}
	return aiInfo, nil
}

// GetSQLDB returns the underlying *sql.DB
func (d *DB) GetSQLDB() *sql.DB {
	return d.db
}

// IsUserAdminInDB проверяет, является ли пользователь администратором в БД
func (d *DB) IsUserAdminInDB(chatID, userID int64) (bool, error) {
	var role string
	query := "SELECT role FROM users_role WHERE chat_id = ? AND user_id = ?"

	// Логируем запрос
	log.Printf("[DB] Checking admin role for user %d in chat %d", userID, chatID)
	log.Printf("[DB] Query: %s", query)

	row := d.db.QueryRow(query, chatID, userID)
	if err := row.Scan(&role); err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[DB] User %d is not admin in chat %d", userID, chatID)
			return false, nil
		}
		log.Printf("[DB] Error checking user role: %v", err)
		return false, fmt.Errorf("error checking user role in DB: %v", err)
	}

	// Логируем результат
	log.Printf("[DB] User %d role in chat %d: %s", userID, chatID, role)

	return role == "admin", nil
}
