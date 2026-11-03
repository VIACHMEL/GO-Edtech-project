package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTables(t *testing.T) {
	source := `<table><tr><th colspan="2">F</th></tr><tr><td></td><td>0</td></tr></table>`
	tables, err := extractTables(cleanHTML(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Rows) != 2 {
		t.Fatalf("таблица повреждена: %+v", tables)
	}
	if tables[0].Rows[0][0].Colspan != 2 {
		t.Fatal("colspan потерян")
	}
	if tables[0].Rows[1][0].Text != "" || tables[0].Rows[1][1].Text != "0" {
		t.Fatal("пустые ячейки не сохранены")
	}
}

func TestSubtaskWithoutDifficulty(t *testing.T) {
	parts, err := prepareSubtasks([]SubTask{{Number: 20, Text: "<p>Условие</p>", Key: "42"}})
	if err != nil {
		t.Fatal(err)
	}
	if parts[0]["difficulty"] != nil {
		t.Fatal("сложность должна быть null")
	}
}

func TestInlineImagePreserved(t *testing.T) {
	// Небольшой Base64 в допустимом формате: проверяем именно сохранение данных,
	// а не распознавание текста на изображении.
	source := `<p>Схема <img src="data:image/png;base64,` +
		`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j8K8AAAAASUVORK5CYII="/></p>`
	cleaned, images, err := prepareHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || !strings.Contains(cleaned, "data:image/png;base64,") {
		t.Fatalf("встроенная картинка потеряна: html=%q, images=%d", cleaned, len(images))
	}
	if isImageOnly(htmlToText(cleaned)) {
		t.Fatal("смешанное условие не должно считаться картинкой без текста")
	}
}

func TestImageOnly(t *testing.T) {
	if !isImageOnly("[изображение]") {
		t.Fatal("чисто графическое условие должно идти в карантин")
	}
	if isImageOnly("Текст [изображение]") {
		t.Fatal("смешанное условие нельзя пропускать")
	}
}

func TestInvalidFiles(t *testing.T) {
	_, err := prepareFiles([]json.RawMessage{json.RawMessage(`""`)})
	if err == nil {
		t.Fatal("пустой URL файла должен выявляться")
	}
}

func TestSourceProvenance(t *testing.T) {
	if sourceYear("Демоверсия 2027") != 2027 {
		t.Fatal("не извлечён год демоверсии")
	}
	if sourceYear("Автор Иван Иванов") != 0 {
		t.Fatal("год без доказательств выдуман")
	}
	task, err := prepareDBTask(31499, KompegeTask{
		Number: 2, Text: "<p>Условие</p>", Key: "yxzw",
		Comment: "Демоверсия 2027", UserID: "vk_123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Year != 2027 || task.Metadata["source_comment"] != "Демоверсия 2027" ||
		task.Metadata["source_uploader_id"] != "vk_123" {
		t.Fatal("потерян источник")
	}
}

func TestMissingAnswerRejected(t *testing.T) {
	task := KompegeTask{Number: 2, Text: "<p>Условие</p>"}
	prepared, err := prepareDBTask(10, task)
	if err != nil {
		t.Fatal(err)
	}
	if taskQualityProblem(task, prepared) == "" {
		t.Fatal("пустой ответ нельзя считать нормальным")
	}
}
