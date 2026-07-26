package sqldbs

import "log"

type RawSQLStore struct {
	stmts map[string]string
}

func NewRawSQLStore() *RawSQLStore {
	return &RawSQLStore{stmts: make(map[string]string)}
}

func (s *RawSQLStore) Set(key string, rawStmt string) {
	s.stmts[key] = rawStmt
}

func (s *RawSQLStore) Get(key string) (string, bool) {
	stmt, exists := s.stmts[key]
	return stmt, exists
}

func (s *RawSQLStore) GetOrPanic(key string) string {
	stmt, exists := s.stmts[key]
	if !exists {
		log.Panicf("raw SQL not found for key: %s", key)
	}
	return stmt
}

func (s *RawSQLStore) GetAll() map[string]string {
	return s.stmts
}
