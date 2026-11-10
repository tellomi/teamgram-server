package thumbbackfill

import (
	"context"
	"database/sql"
	"fmt"
)

const pageSQL = `
SELECT ps.id, ps.photo_size_id, ps.size_type, ps.file_path, ps.width, ps.height, ps.file_size,
       IFNULL(d.mime_type, ''), IFNULL(d.file_path, '')
FROM photo_sizes ps
LEFT JOIN documents d ON d.document_id = ps.photo_size_id
WHERE ps.id > ? AND ps.cached_type = 0 AND ps.file_path <> ''
  AND ps.size_type NOT IN ('i', 'j')
ORDER BY ps.id
LIMIT ?`

// MySQL reads photo_sizes and the document the size was generated from.
type MySQL struct {
	DB *sql.DB
}

func (m MySQL) Next(ctx context.Context, after int64, limit int) ([]Row, error) {
	if m.DB == nil {
		return nil, fmt.Errorf("thumb backfill: mysql is nil")
	}
	rows, err := m.DB.QueryContext(ctx, pageSQL, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var row Row
		if err = rows.Scan(&row.ID, &row.PhotoSizeID, &row.SizeType, &row.Path, &row.Width, &row.Height, &row.FileSize, &row.Mime, &row.DocumentPath); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (m MySQL) UpdateFileSize(ctx context.Context, id int64, size int32) error {
	if m.DB == nil {
		return fmt.Errorf("thumb backfill: mysql is nil")
	}
	_, err := m.DB.ExecContext(ctx, `UPDATE photo_sizes SET file_size = ? WHERE id = ?`, size, id)
	return err
}
