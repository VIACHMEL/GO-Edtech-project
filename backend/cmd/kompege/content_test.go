package main

import "testing"

func TestExtractTables(t *testing.T) {
	source := `<table>
        <tr><th colspan="2">F</th></tr>
        <tr><td></td><td>0</td></tr>
    </table>`

	tables, err := extractTables(cleanHTML(source))
	if err != nil {
		t.Fatal(err)
	}

	if len(tables) != 1 {
		t.Fatalf("ожидали 1 таблицу, получили %d", len(tables))
	}

	rows := tables[0].Rows

	if len(rows) != 2 {
		t.Fatalf("ожидали 2 строки, получили %d", len(rows))
	}

	if rows[0][0].Colspan != 2 {
		t.Fatal("потерян colspan")
	}

	if rows[1][0].Text != "" {
		t.Fatal("потеряна пустая ячейка")
	}

	if rows[1][1].Text != "0" {
		t.Fatal("неправильное значение ячейки")
	}
}

func TestSubtaskWithoutDifficulty(t *testing.T) {
	subtasks, err := prepareSubtasks([]SubTask{
		{Number: 20, Text: "<p>Условие</p>", Key: "42"},
	})

	if err != nil {
		t.Fatal(err)
	}

	if subtasks[0]["difficulty"] != nil {
		t.Fatal("отсутствующая сложность должна быть null")
	}
}
