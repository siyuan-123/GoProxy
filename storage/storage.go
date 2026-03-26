package storage

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Proxy struct {
	ID        int64
	Address   string // host:port
	Protocol  string // http, socks5
	FailCount int
	LastCheck time.Time
	CreatedAt time.Time
}

type Storage struct {
	db *sql.DB
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite 单写

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Storage) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS proxies (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			address    TEXT NOT NULL UNIQUE,
			protocol   TEXT NOT NULL,
			fail_count INTEGER NOT NULL DEFAULT 0,
			last_check DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// AddProxy 新增代理，已存在则忽略
func (s *Storage) AddProxy(address, protocol string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO proxies (address, protocol) VALUES (?, ?)`,
		address, protocol,
	)
	return err
}

// AddProxies 批量新增
func (s *Storage) AddProxies(proxies []Proxy) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO proxies (address, protocol) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, p := range proxies {
		if _, err := stmt.Exec(p.Address, p.Protocol); err != nil {
			log.Printf("insert proxy %s error: %v", p.Address, err)
		}
	}
	return tx.Commit()
}

// GetRandom 随机取一个可用代理
func (s *Storage) GetRandom() (*Proxy, error) {
	rows, err := s.db.Query(
		`SELECT id, address, protocol, fail_count, last_check, created_at
		 FROM proxies WHERE fail_count < 3
		 ORDER BY RANDOM() LIMIT 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		p := &Proxy{}
		var lastCheck sql.NullTime
		if err := rows.Scan(&p.ID, &p.Address, &p.Protocol, &p.FailCount, &lastCheck, &p.CreatedAt); err != nil {
			return nil, err
		}
		if lastCheck.Valid {
			p.LastCheck = lastCheck.Time
		}
		return p, nil
	}
	return nil, fmt.Errorf("no available proxy")
}

// GetAll 获取所有可用代理
func (s *Storage) GetAll() ([]Proxy, error) {
	rows, err := s.db.Query(
		`SELECT id, address, protocol, fail_count, last_check, created_at
		 FROM proxies WHERE fail_count < 3`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []Proxy
	for rows.Next() {
		p := Proxy{}
		var lastCheck sql.NullTime
		if err := rows.Scan(&p.ID, &p.Address, &p.Protocol, &p.FailCount, &lastCheck, &p.CreatedAt); err != nil {
			return nil, err
		}
		if lastCheck.Valid {
			p.LastCheck = lastCheck.Time
		}
		proxies = append(proxies, p)
	}
	return proxies, nil
}

// GetRandomExclude 排除指定地址随机取一个
func (s *Storage) GetRandomExclude(excludes []string) (*Proxy, error) {
	proxies, err := s.GetAll()
	if err != nil {
		return nil, err
	}

	excludeMap := make(map[string]bool)
	for _, e := range excludes {
		excludeMap[e] = true
	}

	var available []Proxy
	for _, p := range proxies {
		if !excludeMap[p.Address] {
			available = append(available, p)
		}
	}

	if len(available) == 0 {
		// 没有可排除的了，随机取任意一个
		return s.GetRandom()
	}

	p := available[rand.Intn(len(available))]
	return &p, nil
}

// Delete 立即删除指定代理
func (s *Storage) Delete(address string) error {
	_, err := s.db.Exec(`DELETE FROM proxies WHERE address = ?`, address)
	return err
}

// IncrFail 增加失败次数
func (s *Storage) IncrFail(address string) error {
	_, err := s.db.Exec(
		`UPDATE proxies SET fail_count = fail_count + 1, last_check = CURRENT_TIMESTAMP WHERE address = ?`,
		address,
	)
	return err
}

// ResetFail 重置失败次数（验证通过）
func (s *Storage) ResetFail(address string) error {
	_, err := s.db.Exec(
		`UPDATE proxies SET fail_count = 0, last_check = CURRENT_TIMESTAMP WHERE address = ?`,
		address,
	)
	return err
}

// DeleteInvalid 删除失败次数超过阈值的代理
func (s *Storage) DeleteInvalid(maxFailCount int) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM proxies WHERE fail_count >= ?`, maxFailCount)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Count 返回可用代理数量
func (s *Storage) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE fail_count < 3`).Scan(&count)
	return count, err
}

// CountByProtocol 按协议统计数量
func (s *Storage) CountByProtocol(protocol string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE fail_count < 3 AND protocol = ?`, protocol).Scan(&count)
	return count, err
}

// GetByProtocol 按协议获取代理列表
func (s *Storage) GetByProtocol(protocol string) ([]Proxy, error) {
	rows, err := s.db.Query(
		`SELECT id, address, protocol, fail_count, last_check, created_at
		 FROM proxies WHERE fail_count < 3 AND protocol = ?
		 ORDER BY created_at DESC`, protocol,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []Proxy
	for rows.Next() {
		p := Proxy{}
		var lastCheck sql.NullTime
		if err := rows.Scan(&p.ID, &p.Address, &p.Protocol, &p.FailCount, &lastCheck, &p.CreatedAt); err != nil {
			return nil, err
		}
		if lastCheck.Valid {
			p.LastCheck = lastCheck.Time
		}
		proxies = append(proxies, p)
	}
	return proxies, nil
}

// Close 关闭数据库
func (s *Storage) Close() error {
	return s.db.Close()
}
