package teacherdata

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func TestToProfileDTO(t *testing.T) {
	user := db.User{
		ID:     5,
		Name:   "Иван Петров",
		Email:  "ivan@example.invalid",
		Role:   "student",
		Status: "active",
	}

	profile := ToProfileDTO(user)

	if profile.ID != 5 || profile.Name != "Иван Петров" {
		t.Fatalf("неожиданный профиль: %+v", profile)
	}

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"name":"Иван Петров"`) {
		t.Fatalf("неожиданный JSON: %s", data)
	}
}

func TestEmptyStudentListJSON(t *testing.T) {
	response := StudentListResponse{
		Students: make([]StudentListItemDTO, 0),
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"students":[]`) {
		t.Fatalf("ожидался пустой массив, получено: %s", data)
	}
}

func TestAvatarLetter(t *testing.T) {
	if got := avatarLetter("Иван Петров"); got != "И" {
		t.Fatalf("ожидалось И, получено %q", got)
	}
}
