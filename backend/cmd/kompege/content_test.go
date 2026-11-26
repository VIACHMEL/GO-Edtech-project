package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSaveTaskMaterials(t *testing.T) {
	client := &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://kompege.ru/files/input.txt" {
			t.Fatalf("неожиданный источник файла: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("8\n13\n"))}, nil
	})}
	task, err := prepareDBTask(5, KompegeTask{
		Number: 24, Key: "8",
		Text:  `<p>Текст <img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j8K8AAAAASUVORK5CYII=" /></p>`,
		Files: []json.RawMessage{json.RawMessage(`{"name":"input.txt","url":"/files/input.txt"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	materials, err := saveTaskMaterials(context.Background(), client, root, 5, task)
	if err != nil {
		t.Fatal(err)
	}
	if len(materials) != 2 || materials[0].mime != "image/png" || materials[1].mime != "text/plain" {
		t.Fatalf("ожидались изображение и текстовый файл: %+v", materials)
	}
	for i, want := range [][]byte{{0x89, 'P', 'N', 'G'}, []byte("8\n13\n")} {
		path := materials[i].filePath
		if !filepath.IsLocal(filepath.FromSlash(path)) {
			t.Fatalf("путь должен быть относительным: %q", path)
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || !bytes.HasPrefix(got, want) {
			t.Fatalf("файл %q потерян или повреждён: %v", path, err)
		}
	}
}

func TestMaterialRedirectCannotLeaveSource(t *testing.T) {
	client := &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "kompege.ru" {
			t.Fatalf("внешний адрес получил запрос: %s", req.URL.Host)
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {"https://example.org/file.txt"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	_, err := materialData(context.Background(), client, "https://kompege.ru/files/input.txt")
	if err == nil || !strings.Contains(err.Error(), "перенаправление") {
		t.Fatalf("внешнее перенаправление должно быть отклонено: %v", err)
	}
}

func TestPrepareExamSubtaskPreservesCommonPrompt(t *testing.T) {
	parent := KompegeTask{ID: 31516, Number: 19, Text: "<p>Правила игры.</p>", Key: "19",
		SubTasks: []SubTask{{Number: 20, Text: "<p>Второй ход.</p>", Key: "42"}}}
	task, err := prepareExamTask(parent, 20)
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskNumber != 20 || task.Answer != "42" ||
		task.Metadata["exam_part"] != 1 ||
		task.SourceURL != "https://kompege.ru/api/v1/task/31516#20" ||
		!strings.Contains(task.ContentText, "Правила игры.") || !strings.Contains(task.ContentText, "Второй ход.") {
		t.Fatalf("потеряна часть условия, ответа или происхождение подзадания")
	}
}

func TestPreparePracticalTaskUsesSeparateBoard(t *testing.T) {
	task, err := prepareExamTask(KompegeTask{ID: 1, Number: 24, Text: "<p>Условие</p>", Key: "42"}, 24)
	if err != nil {
		t.Fatal(err)
	}
	if task.Metadata["exam_part"] != 2 {
		t.Fatalf("для №24 ожидалась отдельная доска")
	}
}

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
