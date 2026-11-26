package db

import "testing"

func TestTeacherStudentSearch(t *testing.T) {
	st := TeacherStudentView{ID: 10, Name: "Иван Петров", Email: "ivan@example.invalid", Groups: []StudentGroupView{{ID: 2, Name: "ЕГЭ информатика"}}}
	cases := []struct {
		query    string
		expected bool
	}{
		{"", true}, {"петров", true}, {"ivan@", true}, {"информат", true}, {"математика", false},
	}
	for _, tc := range cases {
		if got := matchesTeacherStudent(st, tc.query); got != tc.expected {
			t.Errorf("поиск %q: %v != %v", tc.query, got, tc.expected)
		}
	}
}

func TestTeacherGroupSearch(t *testing.T) {
	subject := "Информатика"
	gr := TeacherGroupView{ID: 5, Name: "ЕГЭ", Subject: &subject}
	cases := []struct {
		query    string
		expected bool
	}{
		{"", true}, {"егэ", true}, {"информат", true}, {"математика", false},
	}
	for _, tc := range cases {
		if got := matchesTeacherGroup(gr, tc.query); got != tc.expected {
			t.Errorf("поиск %q: %v != %v", tc.query, got, tc.expected)
		}
	}
}
