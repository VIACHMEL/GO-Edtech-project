package db

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"regexp"

	"github.com/jackc/pgx/v5"
)

const (
	BoardFileLimit      = 2 << 20
	BoardFileTotalLimit = 16 << 20
	BoardFileCountLimit = 64
	boardImageDimension = 4096
	boardImagePixels    = 8 << 20
)

var (
	ErrBoardFileLimit = errors.New("board file capacity exceeded")
	boardFileID       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

func validateBoardFile(id, mime string, data []byte) error {
	if !boardFileID.MatchString(id) || len(data) == 0 || len(data) > BoardFileLimit ||
		(mime != "image/png" && mime != "image/jpeg") {
		return ErrValidation
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || mime != "image/"+format || config.Width < 1 || config.Height < 1 ||
		config.Width > boardImageDimension || config.Height > boardImageDimension ||
		config.Width*config.Height > boardImagePixels {
		return ErrValidation
	}
	return nil
}

// PutBoardFile preserves the content behind an ID once it is visible on a
// board. The same write can be retried after the response is lost.
func (db *DB) PutBoardFile(ctx context.Context, actor, boardID int64, id, mime string, data []byte) error {
	if err := validateBoardFile(id, mime, data); err != nil {
		return err
	}
	return db.learningTx(ctx, actor, "", func(tx pgx.Tx) error {
		editable, err := lockBoardAttempt(ctx, tx, actor, boardID)
		if err != nil {
			return err
		}
		var matches *bool
		var files int
		var total int64
		if err := tx.QueryRow(ctx, `SELECT bool_and(mime=$3 AND content=$4) FILTER (WHERE file_id=$2),
 count(*),COALESCE(sum(octet_length(content)),0)
FROM board_files WHERE board_id=$1`, boardID, id, mime, data).Scan(&matches, &files, &total); err != nil {
			return err
		}
		if matches != nil {
			if *matches {
				return nil
			}
			return ErrConflict
		}
		if !editable {
			return ErrAttemptLocked
		}
		if files >= BoardFileCountLimit || total+int64(len(data)) > BoardFileTotalLimit {
			return ErrBoardFileLimit
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return ErrValidation
		}
		_, err = tx.Exec(ctx, `INSERT INTO board_files(board_id,file_id,mime,content)
VALUES($1,$2,$3,$4)`, boardID, id, mime, data)
		return err
	})
}

func (db *DB) GetBoardFile(ctx context.Context, actor, boardID int64, id string) (string, []byte, error) {
	if !boardFileID.MatchString(id) {
		return "", nil, ErrValidation
	}
	var mime string
	var content []byte
	err := db.Pool.QueryRow(ctx, `SELECT f.mime,f.content`+boardReadAccess+`
JOIN board_files f ON f.board_id=b.id AND f.file_id=$3`+boardReadFilter,
		boardID, actor, id).Scan(&mime, &content)
	return mime, content, worksheetError(err)
}

func checkBoardImages(ctx context.Context, tx pgx.Tx, boardID int64, changes []boardElement) error {
	files := make(map[string]bool)
	for _, element := range changes {
		if element.Type != "image" || element.Deleted {
			continue
		}
		if !boardFileID.MatchString(element.FileID) {
			return ErrValidation
		}
		files[element.FileID] = true
	}
	if len(files) == 0 {
		return nil
	}
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	var found int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM board_files
WHERE board_id=$1 AND file_id=ANY($2::text[])`, boardID, ids).Scan(&found); err != nil {
		return err
	}
	if found != len(ids) {
		return ErrValidation
	}
	return nil
}
