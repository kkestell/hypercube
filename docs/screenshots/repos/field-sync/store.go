package main

import (
	"database/sql"
	"net/http"
	"time"
)

// Store holds devices and the notebook changes they push.
type Store struct {
	db *sql.DB
}

// Device is a signed-in device and when its token was issued.
type Device struct {
	ID     string
	Issued time.Time
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Device(token string) (Device, error) {
	var d Device
	err := s.db.QueryRow(`SELECT id, issued FROM devices WHERE token = ?`, token).Scan(&d.ID, &d.Issued)
	return d, err
}

func (s *Store) ListChanges(w http.ResponseWriter, r *http.Request) {}

func (s *Store) PushChanges(w http.ResponseWriter, r *http.Request) {}
