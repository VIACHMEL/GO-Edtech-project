package teacherdata

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func TestEmptyGroupListJSON(t *testing.T) {
	b, err := json.Marshal(GroupListResponse{Groups: make([]GroupListItemDTO, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"groups":[]`) {
		t.Fatalf("неожиданный JSON: %s", b)
	}
}

func TestGroupItemPublicFields(t *testing.T) {
	item := ToGroupDTO(db.TeacherGroupView{
		ID: 4, Name: "ЕГЭ", Status: "active", Members: []db.TeacherGroupMemberView{{ID: 5, Name: "Мария де ла Крус", Email: "m@example.invalid"}},
	})
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "password") || strings.Contains(got, "token") {
		t.Fatalf("обнаружены секретные данные: %s", got)
	}
	if item.MemberCount != 1 || item.Members[0].Initials != "М" {
		t.Fatalf("неверная структура DTO: %+v", item)
	}
}

func TestAvatarLetterIsNotNameParser(t *testing.T) {
	cases := map[string]string{
		"": "", "  ": "", "Иван Петров": "И", "  анна  Волкова ": "А",
		"Мария де ла Крус": "М", "van der Waals": "V", "Émile Zola": "É",
		"Анна-Мария Петрова": "А", "王小明": "王",
	}
	for input, want := range cases {
		if got := avatarLetter(input); got != want {
			t.Errorf("avatarLetter(%q) = %q, ожидалось %q", input, got, want)
		}
	}
}

func TestStudentDTOEmptyGroups(t *testing.T) {
	student := ToStudentDTO(db.TeacherStudentView{ID: 4, Name: "Пётр", Email: "p@example.invalid", Status: "active"})
	b, err := json.Marshal(student)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"groups":[]`) {
		t.Fatalf("массив групп не должен быть null: %s", b)
	}
}
