// Package storage содержит учебный источник данных в памяти.
package storage

import "context"

// Student — демонстрационный профиль. Настоящие персональные данные не используются.
type Student struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Grade          int    `json:"grade"`
	Subject        string `json:"subject"`
	KnowledgeLevel string `json:"knowledge_level"`
}

type Demo struct{}

func NewDemo() *Demo { return &Demo{} }

// Students возвращает фиктивные данные, а не список реальных учеников.
func (*Demo) Students(_ context.Context) ([]Student, error) {
	return []Student{{
		ID: "demo-1", Name: "Тестовый ученик", Grade: 10,
		Subject: "Математика", KnowledgeLevel: "Повторение уравнений",
	}}, nil
}
