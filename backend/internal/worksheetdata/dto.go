package worksheetdata

import "time"

// краткая информация о варианте (без списка задач)
type WorksheetDTO struct {
	Revision    int64      `json:"revision"`
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Subject     string     `json:"subject"`
	MaxAttempts int        `json:"max_attempts"`
	Status      string     `json:"status"` //"draft", "published", "archive"
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at"` // null, если вариант еще черновик
}

// позиция внутри варианта
type WorksheetItemDTO struct {
	ID            int64          `json:"id"`
	TaskVersionID int64          `json:"task_version_id"`
	Position      int            `json:"position"`
	MaxScore      string         `json:"max_score"` // строка вида "2.50" для точности NUMERIC
	Settings      map[string]any `json:"settings"`  // Дополнительные настройки (например, подсказки или шкала перевода баллов)
}

// детальная информация о варианте
type WorksheetDetailDTO struct {
	WorksheetDTO
	Items []WorksheetItemDTO `json:"items"`
}

// ответ для эндпоинта со списком вариантов
type WorksheetListResponse struct {
	Worksheets []WorksheetDTO `json:"worksheets"`
	Total      int            `json:"total"`
}

// тело запроса для создания нового варианта.
type CreateWorksheetRequest struct {
	Title       string `json:"title"`
	Subject     string `json:"subject"`
	MaxAttempts int    `json:"max_attempts"`
}

// тело запроса для изменения данных варианта
type UpdateWorksheetRequest struct {
	Title       string `json:"title"`
	Subject     string `json:"subject"`
	MaxAttempts int    `json:"max_attempts"`
}

// тело запроса для добавления задачи в вариант.
type AddWorksheetItemRequest struct {
	TaskVersionID int64          `json:"task_version_id"`
	Position      int            `json:"position"`
	MaxScore      string         `json:"max_score"`
	Settings      map[string]any `json:"settings"`
}

// тело запроса для изменения баллов или настроек конкретной позиции.
type UpdateWorksheetItemRequest struct {
	MaxScore string         `json:"max_score"`
	Settings map[string]any `json:"settings"`
}

// тело запроса для изменения порядка задач в варианте.
type ReorderWorksheetItemsRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

// тело запроса для копирования варианта.
type CloneWorksheetRequest struct {
	Title string `json:"title"`
}
