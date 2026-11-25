package worksheetdata

import (
	"context"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type Service struct {
	Store *db.DB
}

func NewService(store *db.DB) *Service {
	return &Service{Store: store}
}

// создает новый вариант в статусе "черновик" (draft)
func (s *Service) CreateWorksheet(ctx context.Context, teacherID int64, req CreateWorksheetRequest) (WorksheetDTO, error) {
	ws, err := s.Store.CreateWorksheet(ctx, teacherID, req.Title, req.Subject, req.MaxAttempts)
	if err != nil {
		return WorksheetDTO{}, err
	}
	return ToWorksheetDTO(ws), nil
}

// получает полную информацию о варианте вместе со списком задач
func (s *Service) GetWorksheet(ctx context.Context, teacherID, id int64) (WorksheetDetailDTO, error) {
	ws, err := s.Store.GetWorksheet(ctx, teacherID, id)
	if err != nil {
		return WorksheetDetailDTO{}, err
	}
	items, err := s.Store.ListWorksheetItems(ctx, teacherID, id)
	if err != nil {
		return WorksheetDetailDTO{}, err
	}
	return ToWorksheetDetailDTO(ws, items), nil
}

// возвращает список вариантов преподавателя (от новых к старым)
func (s *Service) ListWorksheets(ctx context.Context, teacherID, beforeID int64, limit int) (WorksheetListResponse, error) {
	wss, err := s.Store.ListWorksheets(ctx, teacherID, beforeID, limit)
	if err != nil {
		return WorksheetListResponse{}, err
	}
	dtos := make([]WorksheetDTO, 0, len(wss))
	for _, ws := range wss {
		dtos = append(dtos, ToWorksheetDTO(ws))
	}
	return WorksheetListResponse{Worksheets: dtos, Total: len(dtos)}, nil
}

// изменяет название, предмет или лимит попыток черновика
func (s *Service) UpdateWorksheet(ctx context.Context, teacherID, id int64, req UpdateWorksheetRequest) error {
	return s.Store.UpdateWorksheet(ctx, teacherID, id, req.Title, req.Subject, req.MaxAttempts)
}

// публикует черновик
func (s *Service) PublishWorksheet(ctx context.Context, teacherID, id int64) (WorksheetDTO, error) {
	if err := s.Store.PublishWorksheet(ctx, teacherID, id); err != nil {
		return WorksheetDTO{}, err
	}
	ws, err := s.Store.GetWorksheet(ctx, teacherID, id)
	if err != nil {
		return WorksheetDTO{}, err
	}
	return ToWorksheetDTO(ws), nil
}

// перемещает вариант в архив
func (s *Service) ArchiveWorksheet(ctx context.Context, teacherID, id int64) error {
	return s.Store.ArchiveWorksheet(ctx, teacherID, id)
}

// создает полную копию варианта (включая все задачи) в виде нового черновика
func (s *Service) CloneWorksheet(ctx context.Context, teacherID, id int64, req CloneWorksheetRequest) (WorksheetDTO, error) {
	ws, err := s.Store.CloneWorksheet(ctx, teacherID, id, req.Title)
	if err != nil {
		return WorksheetDTO{}, err
	}
	return ToWorksheetDTO(ws), nil
}

// добавляет новую задачу в черновик варианта
func (s *Service) AddWorksheetItem(ctx context.Context, teacherID, id int64, req AddWorksheetItemRequest) (WorksheetItemDTO, error) {
	item := db.WorksheetItem{
		TaskVersionID: req.TaskVersionID,
		Position:      req.Position,
		MaxScore:      req.MaxScore,
		Settings:      req.Settings,
	}
	res, err := s.Store.AddWorksheetItem(ctx, teacherID, id, item)
	if err != nil {
		return WorksheetItemDTO{}, err
	}
	return ToWorksheetItemDTO(res), nil
}

// изменяет баллы или настройки конкретной позиции в черновике
func (s *Service) UpdateWorksheetItem(ctx context.Context, teacherID, id, itemID int64, req UpdateWorksheetItemRequest) error {
	return s.Store.UpdateWorksheetItem(ctx, teacherID, id, itemID, req.MaxScore, req.Settings)
}

// удаляет задачу из черновика
func (s *Service) RemoveWorksheetItem(ctx context.Context, teacherID, id, itemID int64) error {
	return s.Store.RemoveWorksheetItem(ctx, teacherID, id, itemID)
}

// переставляет задачи в нужном порядке
func (s *Service) ReorderWorksheetItems(ctx context.Context, teacherID, id int64, req ReorderWorksheetItemsRequest) error {
	return s.Store.ReorderWorksheetItems(ctx, teacherID, id, req.ItemIDs)
}

// преобразует модель варианта из БД в DTO
func ToWorksheetDTO(ws *db.Worksheet) WorksheetDTO {
	return WorksheetDTO{
		ID:          ws.ID,
		Revision:    ws.Revision,
		Title:       ws.Title,
		Subject:     ws.Subject,
		MaxAttempts: ws.MaxAttempts,
		Status:      ws.Status,
		CreatedAt:   ws.CreatedAt,
		UpdatedAt:   ws.UpdatedAt,
		PublishedAt: ws.PublishedAt,
	}
}

// преобразует модель позиции из БД в DTO
func ToWorksheetItemDTO(item *db.WorksheetItem) WorksheetItemDTO {
	settings := item.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	return WorksheetItemDTO{
		ID:            item.ID,
		TaskVersionID: item.TaskVersionID,
		Position:      item.Position,
		MaxScore:      item.MaxScore,
		Settings:      settings,
	}
}

// собирает детальную информацию о варианте: заголовок + список задач
func ToWorksheetDetailDTO(ws *db.Worksheet, items []*db.WorksheetItem) WorksheetDetailDTO {
	dto := WorksheetDetailDTO{WorksheetDTO: ToWorksheetDTO(ws)}
	dto.Items = make([]WorksheetItemDTO, 0, len(items))
	for _, it := range items {
		dto.Items = append(dto.Items, ToWorksheetItemDTO(it))
	}
	return dto
}
