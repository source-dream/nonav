package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nonav/server/internal/core"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := retryOnSQLiteLock(func() error {
		return store.setupPragmas()
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := retryOnSQLiteLock(func() error {
		return store.migrate()
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func retryOnSQLiteLock(fn func() error) error {
	const maxAttempts = 20
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if !isSQLiteLockedError(err) || attempt == maxAttempts {
			return err
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func isSQLiteLockedError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked")
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			group_name TEXT NOT NULL DEFAULT '',
			icon TEXT NOT NULL DEFAULT '',
			click_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS shares (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL,
			target_url TEXT NOT NULL,
			frp_remote_port INTEGER NOT NULL DEFAULT 0,
			token TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			stopped_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY(site_id) REFERENCES sites(id)
		);`,
		`CREATE TABLE IF NOT EXISTS share_access_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			share_id INTEGER NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			remote_ip TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY(share_id) REFERENCES shares(id)
		);`,
		`CREATE TABLE IF NOT EXISTS share_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			share_id INTEGER NOT NULL,
			token TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY(share_id) REFERENCES shares(id)
		);`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}

	if err := s.ensureShareColumn("frp_remote_port", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	if err := s.ensureShareColumn("share_mode", "TEXT NOT NULL DEFAULT 'path_ctx'"); err != nil {
		return err
	}

	if err := s.ensureShareColumn("subdomain_slug", "TEXT"); err != nil {
		return err
	}

	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_shares_subdomain_slug ON shares(subdomain_slug) WHERE subdomain_slug IS NOT NULL AND subdomain_slug <> ''`); err != nil {
		return fmt.Errorf("create shares subdomain index: %w", err)
	}

	return nil
}

func (s *SQLiteStore) ensureShareColumn(name string, columnDef string) error {
	rows, err := s.db.Query(`PRAGMA table_info(shares);`)
	if err != nil {
		return fmt.Errorf("query shares table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid      int
			colName  string
			colType  string
			notNull  int
			defaultV sql.NullString
			primaryK int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultV, &primaryK); err != nil {
			return fmt.Errorf("scan shares table info: %w", err)
		}

		if colName == name {
			return nil
		}
	}

	if _, err := s.db.Exec(`ALTER TABLE shares ADD COLUMN ` + name + ` ` + columnDef); err != nil {
		return fmt.Errorf("alter shares add %s: %w", name, err)
	}

	return nil
}

func (s *SQLiteStore) setupPragmas() error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA foreign_keys = ON;`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("setup sqlite pragma: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) ListSites(ctx context.Context) ([]core.Site, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, url, group_name, icon, click_count, created_at, updated_at
		FROM sites
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer rows.Close()

	sites := make([]core.Site, 0)
	for rows.Next() {
		var site core.Site
		if err := rows.Scan(
			&site.ID,
			&site.Name,
			&site.URL,
			&site.GroupName,
			&site.Icon,
			&site.ClickCount,
			&site.CreatedAt,
			&site.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}

		sites = append(sites, site)
	}

	return sites, nil
}

func (s *SQLiteStore) CreateSite(ctx context.Context, site core.Site) (core.Site, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sites(name, url, group_name, icon, click_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
	`, site.Name, site.URL, site.GroupName, site.Icon, now, now)
	if err != nil {
		return core.Site{}, fmt.Errorf("create site: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return core.Site{}, fmt.Errorf("site last insert id: %w", err)
	}

	site.ID = id
	site.ClickCount = 0
	site.CreatedAt = now
	site.UpdatedAt = now

	return site, nil
}

func (s *SQLiteStore) UpdateSite(ctx context.Context, site core.Site) (core.Site, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE sites
		SET name = ?, url = ?, group_name = ?, icon = ?, updated_at = ?
		WHERE id = ?
	`, site.Name, site.URL, site.GroupName, site.Icon, now, site.ID)
	if err != nil {
		return core.Site{}, fmt.Errorf("update site: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return core.Site{}, fmt.Errorf("update site rows affected: %w", err)
	}

	if affected == 0 {
		return core.Site{}, ErrNotFound
	}

	updated, err := s.GetSiteByID(ctx, site.ID)
	if err != nil {
		return core.Site{}, err
	}

	return updated, nil
}

func (s *SQLiteStore) DeleteSite(ctx context.Context, siteID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, siteID)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete site rows affected: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *SQLiteStore) IncrementSiteClick(ctx context.Context, siteID int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE sites
		SET click_count = click_count + 1, updated_at = ?
		WHERE id = ?
	`, time.Now().UTC(), siteID)
	if err != nil {
		return fmt.Errorf("increment click: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("increment click rows affected: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *SQLiteStore) GetSiteByID(ctx context.Context, siteID int64) (core.Site, error) {
	var site core.Site
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, url, group_name, icon, click_count, created_at, updated_at
		FROM sites
		WHERE id = ?
	`, siteID).Scan(
		&site.ID,
		&site.Name,
		&site.URL,
		&site.GroupName,
		&site.Icon,
		&site.ClickCount,
		&site.CreatedAt,
		&site.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Site{}, ErrNotFound
		}
		return core.Site{}, fmt.Errorf("get site by id: %w", err)
	}

	return site, nil
}

func (s *SQLiteStore) CreateShare(ctx context.Context, share core.Share, passwordHash string) (core.Share, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO shares(site_id, target_url, share_mode, subdomain_slug, frp_remote_port, token, password_hash, status, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.SiteID, share.TargetURL, share.ShareMode, nullableString(share.Subdomain), share.FRPPort, share.Token, passwordHash, share.Status, share.ExpiresAt, now, now)
	if err != nil {
		return core.Share{}, fmt.Errorf("create share: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return core.Share{}, fmt.Errorf("share last insert id: %w", err)
	}

	share.ID = id
	share.CreatedAt = now
	share.UpdatedAt = now

	return share, nil
}

func (s *SQLiteStore) ListShares(ctx context.Context) ([]core.Share, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			sh.id,
			sh.site_id,
			si.name,
			sh.target_url,
			sh.share_mode,
			COALESCE(sh.subdomain_slug, ''),
			sh.frp_remote_port,
			sh.token,
			sh.status,
			sh.expires_at,
			sh.stopped_at,
			sh.created_at,
			sh.updated_at,
			(SELECT COUNT(1) FROM share_access_logs sal WHERE sal.share_id = sh.id) AS access_hits
		FROM shares sh
		INNER JOIN sites si ON si.id = sh.site_id
		ORDER BY sh.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	defer rows.Close()

	shares := make([]core.Share, 0)
	for rows.Next() {
		var share core.Share
		if err := rows.Scan(
			&share.ID,
			&share.SiteID,
			&share.SiteName,
			&share.TargetURL,
			&share.ShareMode,
			&share.Subdomain,
			&share.FRPPort,
			&share.Token,
			&share.Status,
			&share.ExpiresAt,
			&share.StoppedAt,
			&share.CreatedAt,
			&share.UpdatedAt,
			&share.AccessHits,
		); err != nil {
			return nil, fmt.Errorf("scan share: %w", err)
		}

		shares = append(shares, share)
	}

	return shares, nil
}

func (s *SQLiteStore) GetShareByToken(ctx context.Context, token string) (core.Share, string, error) {
	var share core.Share
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT
			sh.id,
			sh.site_id,
			si.name,
			sh.target_url,
			sh.share_mode,
			COALESCE(sh.subdomain_slug, ''),
			sh.frp_remote_port,
			sh.token,
			sh.status,
			sh.expires_at,
			sh.stopped_at,
			sh.created_at,
			sh.updated_at,
			(SELECT COUNT(1) FROM share_access_logs sal WHERE sal.share_id = sh.id) AS access_hits,
			sh.password_hash
		FROM shares sh
		INNER JOIN sites si ON si.id = sh.site_id
		WHERE sh.token = ?
	`, token).Scan(
		&share.ID,
		&share.SiteID,
		&share.SiteName,
		&share.TargetURL,
		&share.ShareMode,
		&share.Subdomain,
		&share.FRPPort,
		&share.Token,
		&share.Status,
		&share.ExpiresAt,
		&share.StoppedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.AccessHits,
		&passwordHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Share{}, "", ErrNotFound
		}
		return core.Share{}, "", fmt.Errorf("get share by token: %w", err)
	}

	return share, passwordHash, nil
}

func (s *SQLiteStore) GetShareByID(ctx context.Context, shareID int64) (core.Share, string, error) {
	var share core.Share
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT
			sh.id,
			sh.site_id,
			si.name,
			sh.target_url,
			sh.share_mode,
			COALESCE(sh.subdomain_slug, ''),
			sh.frp_remote_port,
			sh.token,
			sh.status,
			sh.expires_at,
			sh.stopped_at,
			sh.created_at,
			sh.updated_at,
			(SELECT COUNT(1) FROM share_access_logs sal WHERE sal.share_id = sh.id) AS access_hits,
			sh.password_hash
		FROM shares sh
		INNER JOIN sites si ON si.id = sh.site_id
		WHERE sh.id = ?
	`, shareID).Scan(
		&share.ID,
		&share.SiteID,
		&share.SiteName,
		&share.TargetURL,
		&share.ShareMode,
		&share.Subdomain,
		&share.FRPPort,
		&share.Token,
		&share.Status,
		&share.ExpiresAt,
		&share.StoppedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.AccessHits,
		&passwordHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Share{}, "", ErrNotFound
		}
		return core.Share{}, "", fmt.Errorf("get share by id: %w", err)
	}

	return share, passwordHash, nil
}

func (s *SQLiteStore) ListSharesBySite(ctx context.Context, siteID int64) ([]core.Share, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			sh.id,
			sh.site_id,
			si.name,
			sh.target_url,
			sh.share_mode,
			COALESCE(sh.subdomain_slug, ''),
			sh.frp_remote_port,
			sh.token,
			sh.status,
			sh.expires_at,
			sh.stopped_at,
			sh.created_at,
			sh.updated_at,
			(SELECT COUNT(1) FROM share_access_logs sal WHERE sal.share_id = sh.id) AS access_hits
		FROM shares sh
		INNER JOIN sites si ON si.id = sh.site_id
		WHERE sh.site_id = ?
		ORDER BY sh.created_at DESC
	`, siteID)
	if err != nil {
		return nil, fmt.Errorf("list shares by site: %w", err)
	}
	defer rows.Close()

	shares := make([]core.Share, 0)
	for rows.Next() {
		var share core.Share
		if err := rows.Scan(
			&share.ID,
			&share.SiteID,
			&share.SiteName,
			&share.TargetURL,
			&share.ShareMode,
			&share.Subdomain,
			&share.FRPPort,
			&share.Token,
			&share.Status,
			&share.ExpiresAt,
			&share.StoppedAt,
			&share.CreatedAt,
			&share.UpdatedAt,
			&share.AccessHits,
		); err != nil {
			return nil, fmt.Errorf("scan share by site: %w", err)
		}

		shares = append(shares, share)
	}

	return shares, nil
}

func (s *SQLiteStore) StopShare(ctx context.Context, shareID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin stop share tx: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM shares WHERE id = ?`, shareID).Scan(&count); err != nil {
		return fmt.Errorf("query share before stop: %w", err)
	}

	if count == 0 {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM share_sessions WHERE share_id = ?`, shareID); err != nil {
		return fmt.Errorf("delete share sessions: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM share_access_logs WHERE share_id = ?`, shareID); err != nil {
		return fmt.Errorf("delete share access logs: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM shares WHERE id = ?`, shareID); err != nil {
		return fmt.Errorf("delete share: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stop share tx: %w", err)
	}

	return nil
}

func (s *SQLiteStore) DeleteSharesBySite(ctx context.Context, siteID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete shares by site tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM share_sessions
		WHERE share_id IN (SELECT id FROM shares WHERE site_id = ?)
	`, siteID); err != nil {
		return fmt.Errorf("delete site share sessions: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM share_access_logs
		WHERE share_id IN (SELECT id FROM shares WHERE site_id = ?)
	`, siteID); err != nil {
		return fmt.Errorf("delete site share access logs: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM shares WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("delete site shares: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete shares by site tx: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetUsedFRPPorts(ctx context.Context) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT frp_remote_port
		FROM shares
		WHERE status = 'active' AND frp_remote_port > 0
	`)
	if err != nil {
		return nil, fmt.Errorf("list used frp ports: %w", err)
	}
	defer rows.Close()

	ports := make(map[int]struct{})
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, fmt.Errorf("scan used frp port: %w", err)
		}
		ports[port] = struct{}{}
	}

	return ports, nil
}

func (s *SQLiteStore) ListExpiredActiveShareSiteIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT site_id
		FROM shares
		WHERE status = 'active' AND expires_at < ? AND frp_remote_port > 0
	`, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("list expired active share site ids: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var siteID int64
		if err := rows.Scan(&siteID); err != nil {
			return nil, fmt.Errorf("scan expired active share site id: %w", err)
		}
		ids = append(ids, siteID)
	}

	return ids, nil
}

func (s *SQLiteStore) LogShareAccess(ctx context.Context, shareID int64, method string, path string, remoteIP string, statusCode int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO share_access_logs(share_id, method, path, remote_ip, status_code, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, shareID, method, path, remoteIP, statusCode, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("log share access: %w", err)
	}

	return nil
}

func (s *SQLiteStore) CreateShareSession(ctx context.Context, shareID int64, sessionToken string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO share_sessions(share_id, token, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, shareID, sessionToken, expiresAt.UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create share session: %w", err)
	}

	return nil
}

func (s *SQLiteStore) ValidateShareSession(ctx context.Context, shareID int64, sessionToken string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM share_sessions
		WHERE share_id = ?
			AND token = ?
			AND expires_at >= ?
	`, shareID, sessionToken, time.Now().UTC()).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate share session: %w", err)
	}

	return count > 0, nil
}

func (s *SQLiteStore) GetShareBySubdomain(ctx context.Context, slug string) (core.Share, string, error) {
	var share core.Share
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT
			sh.id,
			sh.site_id,
			si.name,
			sh.target_url,
			sh.share_mode,
			COALESCE(sh.subdomain_slug, ''),
			sh.frp_remote_port,
			sh.token,
			sh.status,
			sh.expires_at,
			sh.stopped_at,
			sh.created_at,
			sh.updated_at,
			(SELECT COUNT(1) FROM share_access_logs sal WHERE sal.share_id = sh.id) AS access_hits,
			sh.password_hash
		FROM shares sh
		INNER JOIN sites si ON si.id = sh.site_id
		WHERE sh.subdomain_slug = ?
		LIMIT 1
	`, slug).Scan(
		&share.ID,
		&share.SiteID,
		&share.SiteName,
		&share.TargetURL,
		&share.ShareMode,
		&share.Subdomain,
		&share.FRPPort,
		&share.Token,
		&share.Status,
		&share.ExpiresAt,
		&share.StoppedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.AccessHits,
		&passwordHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Share{}, "", ErrNotFound
		}
		return core.Share{}, "", fmt.Errorf("get share by subdomain: %w", err)
	}

	return share, passwordHash, nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func (s *SQLiteStore) PurgeExpiredShares(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin purge expired shares tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM share_sessions
		WHERE share_id IN (
			SELECT id FROM shares WHERE status = 'active' AND expires_at < ?
		)
	`, time.Now().UTC()); err != nil {
		return fmt.Errorf("purge expired share sessions by share: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM share_access_logs
		WHERE share_id IN (
			SELECT id FROM shares WHERE status = 'active' AND expires_at < ?
		)
	`, time.Now().UTC()); err != nil {
		return fmt.Errorf("purge expired share logs by share: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM shares
		WHERE status = 'active' AND expires_at < ?
	`, time.Now().UTC()); err != nil {
		return fmt.Errorf("purge expired shares: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM share_sessions WHERE expires_at < ?`, time.Now().UTC()); err != nil {
		return fmt.Errorf("purge expired sessions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit purge expired shares tx: %w", err)
	}

	return nil
}
