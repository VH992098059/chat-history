package repositories

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wangle201210/chat-history/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// InitDB 初始化数据库连接
// dsn 格式：
// - MySQL: user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
// - SQLite: /path/to/file.db 或 /path/to/file.db?_journal_mode=WAL
func InitDB(dsn string) error {
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	// 根据 DSN 格式判断数据库类型
	var dialector gorm.Dialector
	var dbType string
	var maxOpenConns, maxIdleConns int

	if isPostgresDSN(dsn) {
		dbType = "postgres"
		dialector = postgres.Open(dsn)
		maxOpenConns = 100
		maxIdleConns = 10
	} else if strings.Contains(dsn, "@tcp(") {
		// MySQL DSN 格式
		dbType = "mysql"
		dsn = ensureMySQLConfig(dsn)
		dialector = mysql.Open(dsn)
		maxOpenConns = 100
		maxIdleConns = 10
	} else {
		// SQLite DSN 格式（文件路径）
		dbType = "sqlite"
		dialector = sqlite.Open(dsn)
		maxOpenConns = 1
		maxIdleConns = 1
	}

	var err error
	db, err = gorm.Open(dialector, config)
	if err != nil {
		return fmt.Errorf("failed to connect database (%s): %v", dbType, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %v", err)
	}

	// 设置连接池
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移数据库表结构
	if err = autoMigrateTables(dbType); err != nil {
		return fmt.Errorf("failed to migrate database tables: %v", err)
	}

	return nil
}

func isPostgresDSN(dsn string) bool {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return true
	}
	return strings.Contains(dsn, "host=") && strings.Contains(dsn, "dbname=")
}

func ensureMySQLConfig(dsn string) string {
	if !strings.Contains(dsn, "?") {
		return dsn + "?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=True&loc=Local"
	}

	questionIndex := strings.Index(dsn, "?")
	base := dsn[:questionIndex]
	params := dsn[questionIndex+1:]

	if len(params) == 0 {
		return base + "?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=True&loc=Local"
	}

	parts := strings.Split(params, "&")
	hasCharset := false
	hasCollation := false
	hasParseTime := false
	hasLoc := false

	for i, part := range parts {
		if strings.HasPrefix(part, "charset=") {
			parts[i] = "charset=utf8mb4"
			hasCharset = true
		} else if strings.HasPrefix(part, "collation=") {
			parts[i] = "collation=utf8mb4_unicode_ci"
			hasCollation = true
		} else if strings.HasPrefix(part, "parseTime=") {
			hasParseTime = true
		} else if strings.HasPrefix(part, "loc=") {
			hasLoc = true
		}
	}

	if !hasCharset {
		parts = append(parts, "charset=utf8mb4")
	}
	if !hasCollation {
		parts = append(parts, "collation=utf8mb4_unicode_ci")
	}
	if !hasParseTime {
		parts = append(parts, "parseTime=True")
	}
	if !hasLoc {
		parts = append(parts, "loc=Local")
	}

	return base + "?" + strings.Join(parts, "&")
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	if db == nil {
		log.Fatal("database connection not initialized")
	}
	return db
}

// autoMigrateTables 自动迁移数据库表结构
func autoMigrateTables(dbType string) error {
	migrator := db
	if dbType == "mysql" {
		migrator = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	}

	// 自动迁移会创建表、缺失的外键、约束、列和索引
	return migrator.AutoMigrate(
		&models.Conversation{},
		&models.Message{},
		&models.Attachment{},
		&models.MessageAttachment{},
	)
}
