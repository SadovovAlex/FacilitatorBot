package module

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// Captcha представляет структуру капчи
type Captcha struct {
	ID         int64
	ChatID     int64
	UserID     int64
	Question   string
	Answer     int
	SentAt     time.Time
	AnsweredAt *time.Time
	IsCorrect  bool
}

// CaptchaManager управляет операциями с капчей
type CaptchaManager struct {
	db *sql.DB
}

// NewCaptchaManager создает новый менеджер капчи
func NewCaptchaManager(db *sql.DB) *CaptchaManager {
	return &CaptchaManager{db: db}
}

// SendCaptcha генерирует и отправляет капчу пользователю
func (cm *CaptchaManager) SendCaptcha(chatID, userID int64) (string, *Captcha, error) {
	// Генерируем простое математическое выражение
	question, answer := cm.generateSimpleMathQuestion()

	captcha := &Captcha{
		ChatID:   chatID,
		UserID:   userID,
		Question: question,
		Answer:   answer,
		SentAt:   time.Now(),
	}

	// Сохраняем капчу в базу данных
	id, err := cm.saveCaptcha(captcha)
	if err != nil {
		return "", nil, err
	}
	captcha.ID = id

	// Формируем текст для отправки
	text := fmt.Sprintf("Сколько будет: %s = ?", question)

	return text, captcha, nil
}

// VerifyCaptcha проверяет ответ пользователя
func (cm *CaptchaManager) VerifyCaptcha(chatID, userID int64, userAnswer string) (bool, error) {
	// Получаем активную капчу для пользователя
	captcha, err := cm.getActiveCaptcha(chatID, userID)
	if err != nil {
		return false, err
	}

	if captcha == nil {
		return false, fmt.Errorf("активная капча не найдена")
	}

	// Проверяем, не просрочена ли капча (5 минут)
	if time.Since(captcha.SentAt) > 5*time.Minute {
		return false, fmt.Errorf("время для решения капчи истекло")
	}

	// Парсим ответ пользователя
	userAnswer = strings.TrimSpace(userAnswer)
	answerInt, err := strconv.Atoi(userAnswer)
	if err != nil {
		return false, nil // Неверный формат ответа
	}

	// Проверяем ответ
	isCorrect := answerInt == captcha.Answer

	// Обновляем запись капчи
	err = cm.updateCaptchaAnswer(captcha.ID, isCorrect)
	if err != nil {
		return false, err
	}

	return isCorrect, nil
}

// HasActiveCaptcha проверяет наличие активной капчи у пользователя
func (cm *CaptchaManager) HasActiveCaptcha(chatID, userID int64) (*Captcha, error) {
	captcha, err := cm.getActiveCaptcha(chatID, userID)
	if err != nil {
		return nil, err
	}
	return captcha, nil
}

// generateSimpleMathQuestion генерирует простую математическую задачу
func (cm *CaptchaManager) generateSimpleMathQuestion() (string, int) {
	operations := []string{"+", "-"}
	operation := operations[rand.Intn(len(operations))]

	var a, b, answer int

	switch operation {
	case "+":
		a = rand.Intn(50) + 1 // 1-50
		b = rand.Intn(50) + 1 // 1-50
		answer = a + b
	case "-":
		a = rand.Intn(50) + 20 // 20-70
		b = rand.Intn(a) + 1   // 1-a
		answer = a - b
	}

	question := fmt.Sprintf("%d %s %d", a, operation, b)
	return question, answer
}

// saveCaptcha сохраняет капчу в базу данных
func (cm *CaptchaManager) saveCaptcha(captcha *Captcha) (int64, error) {
	query := `
		INSERT INTO captchas (chat_id, user_id, question, answer, sent_at) 
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := cm.db.Exec(query, captcha.ChatID, captcha.UserID, captcha.Question, captcha.Answer, captcha.SentAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// hasUserPassedCaptcha Проверка истории капчи в БД
func (cm *CaptchaManager) HasUserPassedCaptcha(chatID, userID int64) (bool, error) {
	query := `
		SELECT COUNT(*) FROM captchas 
		WHERE chat_id = ? AND user_id = ? AND is_correct = TRUE
		LIMIT 1
	`
	var count int
	err := cm.db.QueryRow(query, chatID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// getActiveCaptcha получает активную капчу для пользователя
func (cm *CaptchaManager) getActiveCaptcha(chatID, userID int64) (*Captcha, error) {
	query := `
		SELECT id, chat_id, user_id, question, answer, sent_at, answered_at, is_correct 
		FROM captchas 
		WHERE chat_id = ? AND user_id = ? AND answered_at IS NULL 
		ORDER BY sent_at DESC LIMIT 1
	`

	row := cm.db.QueryRow(query, chatID, userID)
	var captcha Captcha
	var answeredAt sql.NullTime

	err := row.Scan(&captcha.ID, &captcha.ChatID, &captcha.UserID, &captcha.Question,
		&captcha.Answer, &captcha.SentAt, &answeredAt, &captcha.IsCorrect)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Капча не найдена
		}
		return nil, err
	}

	if answeredAt.Valid {
		captcha.AnsweredAt = &answeredAt.Time
	}
	return &captcha, nil
}

// updateCaptchaAnswer обновляет ответ на капчу
func (cm *CaptchaManager) updateCaptchaAnswer(captchaID int64, isCorrect bool) error {
	answeredAt := time.Now()
	query := `
		UPDATE captchas 
		SET answered_at = ?, is_correct = ? 
		WHERE id = ?
	`
	_, err := cm.db.Exec(query, answeredAt, isCorrect, captchaID)
	return err
}

// GetCaptchaMigration возвращает миграцию для таблицы капчи
func GetCaptchaMigration() struct {
	name string
	sql  string
} {
	return struct {
		name string
		sql  string
	}{
		name: "add_captchas_table",
		sql: `
			CREATE TABLE IF NOT EXISTS captchas (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				chat_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				question TEXT NOT NULL,
				answer INTEGER NOT NULL,
				sent_at TIMESTAMP NOT NULL,
				answered_at TIMESTAMP NULL,
				is_correct BOOLEAN DEFAULT FALSE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id)
			);

			CREATE INDEX IF NOT EXISTS idx_captchas_chat_user ON captchas(chat_id, user_id);
			CREATE INDEX IF NOT EXISTS idx_captchas_sent_at ON captchas(sent_at);
			CREATE INDEX IF NOT EXISTS idx_captchas_active ON captchas(chat_id, user_id, answered_at);
		`,
	}
}
