package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emcfarlane/larking/examples/library/apipb"
)

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS books (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	shelf_id TEXT NOT NULL,
	title TEXT NOT NULL,
	author TEXT NOT NULL,
	cover_image TEXT NOT NULL,
	create_time TIMESTAMP NOT NULL,
	update_time TIMESTAMP NOT NULL
);`)
	return err
}

func (s *Server) GetBook(ctx context.Context, req *apipb.GetBookRequest) (*apipb.Book, error) {
	ids := strings.Split(req.Name, "/")
	shelfID, bookID := ids[1], ids[3]
	id, err := strconv.Atoi(bookID)
	if err != nil {
		return nil, err
	}

	var (
		book = apipb.Book{Name: req.Name}
	)
	if err := s.db.QueryRowContext(ctx, `
SELECT title, author, cover_image 
FROM books WHERE shelf_id = ? AND id = ?
`, shelfID, id).Scan(
		&book.Title, &book.Author, &book.CoverImage,
	); err != nil {
		return nil, err
	}
	return &book, nil

}
func (s *Server) CreateBook(ctx context.Context, req *apipb.CreateBookRequest) (*apipb.Book, error) {
	t := time.Now()

	ids := strings.Split(req.Parent, "/")
	shelfID := ids[1]

	var id int
	if err := s.db.QueryRowContext(ctx, `
INSERT INTO
	books(shelf_id, title, author, cover_image, create_time, update_time)
VALUES (?,?,?,?,?,?)
RETURNING id
`, shelfID, req.Book.Title, req.Book.Author, req.Book.CoverImage, t, t,
	).Scan(&id); err != nil {
		return nil, err
	}
	req.Book.Name = fmt.Sprintf("%s/books/%d", req.Parent, id)
	return req.Book, nil
}
