package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db"
)

type record struct {
	SourceURL   string         `json:"source_url"`
	SourceSite  string         `json:"source_site"`
	Subject     string         `json:"subject"`
	ExamType    string         `json:"exam_type"`
	TaskNumber  int            `json:"task_number"`
	TaskType    string         `json:"task_type"`
	ContentText string         `json:"content_text"`
	Answer      string         `json:"answer"`
	Metadata    map[string]any `json:"metadata"`
}

func run() error {
	file := flag.String("file", "", "JSON-файл с заданиями")
	write := flag.Bool("write", false, "записать задания в БД из DATABASE_URL")
	flag.Parse()
	if *file == "" {
		return fmt.Errorf("укажите --file")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("JSON: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("нет заданий для импорта")
	}
	tasks := make([]*db.Task, 0, len(records))
	for i, r := range records {
		u, err := url.Parse(r.SourceURL)
		if err != nil || u.Scheme != "https" || u.Hostname() != "3.shkolkovo.online" || r.SourceSite != "3.shkolkovo.online" || r.Subject != "math" || r.ExamType != "oge" || r.TaskNumber < 1 || r.TaskNumber > 25 || strings.TrimSpace(r.ContentText) == "" || strings.TrimSpace(r.Answer) == "" {
			return fmt.Errorf("строка %d: запись не соответствует контракту ОГЭ", i+1)
		}
		tasks = append(tasks, &db.Task{SourceURL: r.SourceURL, SourceSite: r.SourceSite, Subject: r.Subject, ExamType: r.ExamType, TaskNumber: r.TaskNumber, TaskType: r.TaskType, ContentText: r.ContentText, Answer: r.Answer, Metadata: r.Metadata})
	}
	if !*write {
		fmt.Printf("Проверено заданий: %d. В БД ничего не записано.\n", len(tasks))
		return nil
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("для записи задайте DATABASE_URL")
	}
	ctx := context.Background()
	store, err := db.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.AddTasks(ctx, tasks); err != nil {
		return fmt.Errorf("сохранение заданий: %w", err)
	}
	fmt.Printf("Обработано заданий: %d\n", len(tasks))
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}
