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

		// 尝试创建 MySQL 数据库
		if err := createMySQLDatabase(dsn); err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}

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

func createMySQLDatabase(dsn string) error {
	// 解析 DSN 获取数据库名
	// user:pass@tcp(host:port)/dbname?charset=utf8mb4...
	parts := strings.Split(dsn, "/")
	if len(parts) < 2 {
		return nil // 无法解析出数据库名，不做处理
	}

	// 截取掉数据库名后面的参数部分
	dbNameWithParams := parts[1]

	// 如果包含 ?，说明有参数
	if strings.Contains(dbNameWithParams, "?") {
		// 截取参数
		paramIndex := strings.Index(dbNameWithParams, "?")
		dbNameWithParams = dbNameWithParams[:paramIndex]
	}

	if dbNameWithParams == "" {
		return nil
	}

	// 连接到 MySQL 服务器（不指定数据库）
	// user:pass@tcp(host:port)/
	baseDSN := parts[0] + "/"
	db, err := gorm.Open(mysql.Open(baseDSN), &gorm.Config{})
	if err != nil {
		return nil // 连接失败可能是没权限等原因，忽略错误，让后面的流程处理
	}

	// 创建数据库
	createSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` DEFAULT CHARACTER SET utf8mb4 DEFAULT COLLATE utf8mb4_unicode_ci;", dbNameWithParams)
	// 忽略错误，因为可能是权限不足，如果数据库不存在后续连接会报错
	_ = db.Exec(createSQL)

	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}

	return nil
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
