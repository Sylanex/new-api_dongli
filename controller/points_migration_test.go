package controller

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestPointsMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "points_migration_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldRedis, oldQuotaPerUnit := common.RedisEnabled, common.QuotaPerUnit
			oldSettings := *operation_setting.GetPointsMigrationSetting()
			model.DB, model.LOG_DB = db, db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			common.SetLogDatabaseType(common.DatabaseType(dialect))
			common.RedisEnabled, common.QuotaPerUnit = false, 500000
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&model.TopUp{}, &model.Log{}, &model.Token{}, &model.User{}))
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetMainDatabaseType(oldType)
				common.SetLogDatabaseType(oldLogType)
				common.RedisEnabled, common.QuotaPerUnit = oldRedis, oldQuotaPerUnit
				*operation_setting.GetPointsMigrationSetting() = oldSettings
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Token{}))
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)
			cutover := time.Now().Add(-time.Hour).Unix()

			t.Run("upgrading existing orders preserves the original amount and unique order number", func(t *testing.T) {
				require.NoError(t, db.Migrator().DropColumn(&model.TopUp{}, "CreditedQuota"))
				require.NoError(t, db.Migrator().DropColumn(&model.User{}, "LegacyUsedQuota"))
				require.NoError(t, db.Migrator().DropColumn(&model.Token{}, "LegacyUsedQuota"))
				oldUser := model.User{Username: "legacy_used", AffCode: "legacy_used", UsedQuota: 500000}
				require.NoError(t, db.Omit("LegacyUsedQuota").Create(&oldUser).Error)
				oldKey := model.Token{UserId: oldUser.Id, Key: "legacy-points-key", UsedQuota: 500000}
				require.NoError(t, db.Omit("LegacyUsedQuota").Create(&oldKey).Error)
				order := model.TopUp{Amount: 1, Money: 7.33, TradeNo: "points_legacy_schema", Status: common.TopUpStatusPending, PaymentProvider: model.PaymentProviderEpay}
				require.NoError(t, db.Omit("CreditedQuota").Create(&order).Error)
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.User{}, &model.Token{}))
				}
				var after model.TopUp
				require.NoError(t, db.First(&after, order.Id).Error)
				assert.Equal(t, order.Amount, after.Amount)
				assert.Equal(t, order.Money, after.Money)
				assert.Zero(t, after.CreditedQuota)
				require.NoError(t, db.First(&oldUser, oldUser.Id).Error)
				require.NoError(t, db.First(&oldKey, oldKey.Id).Error)
				assert.Nil(t, oldUser.LegacyUsedQuota)
				assert.Nil(t, oldKey.LegacyUsedQuota)
				assert.Equal(t, 500000, oldUser.UsedQuota)
				assert.Equal(t, 500000, oldKey.UsedQuota)
				require.NoError(t, db.Model(&oldUser).Update("legacy_used_quota", oldUser.UsedQuota).Error)
				require.NoError(t, db.Model(&oldKey).Update("legacy_used_quota", oldKey.UsedQuota).Error)
				require.NoError(t, db.First(&oldUser, oldUser.Id).Error)
				require.NoError(t, db.First(&oldKey, oldKey.Id).Error)
				require.NotNil(t, oldUser.LegacyUsedQuota)
				require.NotNil(t, oldKey.LegacyUsedQuota)
				assert.Equal(t, oldUser.UsedQuota, *oldUser.LegacyUsedQuota)
				assert.Equal(t, oldKey.UsedQuota, *oldKey.LegacyUsedQuota)
				profile, err := model.GetSelfUserById(oldUser.Id)
				require.NoError(t, err)
				require.NotNil(t, profile.LegacyUsedQuota)
				assert.Equal(t, oldUser.UsedQuota, *profile.LegacyUsedQuota)
				duplicate := model.TopUp{TradeNo: order.TradeNo}
				assert.Error(t, db.Create(&duplicate).Error)
			})

			t.Run("post-cutover zero-usage users and keys start with a zero legacy baseline", func(t *testing.T) {
				operation_setting.GetPointsMigrationSetting().CutoverTime = cutover
				user := model.User{Username: "points_new", AffCode: "points_new"}
				require.NoError(t, db.Create(&user).Error)
				key := model.Token{UserId: user.Id, Key: "new-points-key"}
				require.NoError(t, db.Create(&key).Error)
				require.NotNil(t, user.LegacyUsedQuota)
				require.NotNil(t, key.LegacyUsedQuota)
				assert.Zero(t, *user.LegacyUsedQuota)
				assert.Zero(t, *key.LegacyUsedQuota)
				operation_setting.GetPointsMigrationSetting().CutoverTime = 0
			})

			cases := []struct {
				name         string
				cutover      int64
				createdAt    int64
				provider     string
				method       string
				money        float64
				manual       bool
				initialQuota int
				wantQuota    int
				wantQuote    string
				wantError    error
			}{
				{name: "restriction inactive preserves old credits", provider: model.PaymentProviderEpay, method: "alipay", money: 7.33, wantQuota: 500000},
				{name: "future cutover preserves old credits", cutover: time.Now().Add(time.Hour).Unix(), provider: model.PaymentProviderEpay, method: "alipay", money: 7.33, wantQuota: 500000},
				{name: "old epay callback credits the paid CNY including cents", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", money: 7.33, wantQuota: 3665000},
				{name: "old epay uses the gateway two-decimal quote", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", money: 6.935, wantQuota: 3465000, wantQuote: "6.93"},
				{name: "new order uses its quoted credit amount", cutover: cutover, createdAt: cutover, provider: model.PaymentProviderEpay, method: "wxpay", money: 7.33, wantQuota: 500000},
				{name: "manual old epay completion uses the paid CNY", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "wxpay", money: 7.33, manual: true, wantQuota: 3665000},
				{name: "manual legacy provider with known method uses the paid CNY", cutover: cutover, createdAt: cutover - 1, method: "wechat", money: 7.33, manual: true, wantQuota: 3665000},
				{name: "ambiguous legacy payment remains pending for review", cutover: cutover, createdAt: cutover - 1, money: 7.33, manual: true, wantError: model.ErrLegacyTopUpNeedsReview},
				{name: "zero paid money leaves order and wallet unchanged", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", wantError: model.ErrInvalidTopUpQuota},
				{name: "negative paid money leaves order and wallet unchanged", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", money: -1, wantError: model.ErrInvalidTopUpQuota},
				{name: "oversized paid money cannot overflow credit", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", money: 1e20, wantError: model.ErrInvalidTopUpQuota},
				{name: "wallet ceiling rolls back completion and credited quota", cutover: cutover, createdAt: cutover - 1, provider: model.PaymentProviderEpay, method: "alipay", money: 7.33, initialQuota: common.MaxWalletQuota - 1, wantError: model.ErrTopUpQuotaLimitExceeded},
			}
			for index, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					operation_setting.GetPointsMigrationSetting().CutoverTime = tc.cutover
					user := model.User{Username: fmt.Sprintf("points_%d", index), AffCode: fmt.Sprintf("points_%d", index), Quota: tc.initialQuota}
					require.NoError(t, db.Create(&user).Error)
					order := model.TopUp{UserId: user.Id, Amount: 1, Money: tc.money, TradeNo: fmt.Sprintf("points_order_%d", index), CreateTime: tc.createdAt, PaymentProvider: tc.provider, PaymentMethod: tc.method, Status: common.TopUpStatusPending}
					require.NoError(t, db.Create(&order).Error)
					if tc.manual {
						err = model.ManualCompleteTopUp(order.TradeNo, "127.0.0.1")
					} else {
						_, err = model.RechargeEpay(order.TradeNo, tc.method, "127.0.0.1")
					}
					var afterUser model.User
					var afterOrder model.TopUp
					require.NoError(t, db.First(&afterUser, user.Id).Error)
					require.NoError(t, db.First(&afterOrder, order.Id).Error)
					assert.Equal(t, order.Amount, afterOrder.Amount)
					assert.Equal(t, order.Money, afterOrder.Money)
					if tc.wantQuote != "" {
						assert.Equal(t, tc.wantQuote, afterOrder.QuotedMoney)
					}
					if tc.wantError != nil {
						require.ErrorIs(t, err, tc.wantError)
						assert.Equal(t, tc.initialQuota, afterUser.Quota)
						assert.Equal(t, common.TopUpStatusPending, afterOrder.Status)
						assert.Zero(t, afterOrder.CreditedQuota)
						assert.Zero(t, afterOrder.CompleteTime)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, tc.initialQuota+tc.wantQuota, afterUser.Quota)
					assert.EqualValues(t, tc.wantQuota, afterOrder.CreditedQuota)
					assert.Equal(t, common.TopUpStatusSuccess, afterOrder.Status)
					if tc.manual {
						require.NoError(t, model.ManualCompleteTopUp(order.TradeNo, "127.0.0.1"))
					} else {
						done, err := model.RechargeEpay(order.TradeNo, tc.method, "127.0.0.1")
						require.NoError(t, err)
						assert.True(t, done)
					}
					require.NoError(t, db.First(&afterUser, user.Id).Error)
					assert.Equal(t, tc.initialQuota+tc.wantQuota, afterUser.Quota)
				})
			}

			t.Run("concurrent duplicate callbacks credit an old order once", func(t *testing.T) {
				operation_setting.GetPointsMigrationSetting().CutoverTime = cutover
				if dialect != "sqlite" {
					sqlDB.SetMaxOpenConns(4)
				}
				user := model.User{Username: "points_concurrent", AffCode: "points_concurrent"}
				require.NoError(t, db.Create(&user).Error)
				order := model.TopUp{UserId: user.Id, Amount: 1, Money: 7.33, TradeNo: "points_concurrent", CreateTime: cutover - 1, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}
				require.NoError(t, db.Create(&order).Error)
				var group sync.WaitGroup
				errors := make(chan error, 4)
				for range 4 {
					group.Go(func() { _, err := model.RechargeEpay(order.TradeNo, "alipay", "127.0.0.1"); errors <- err })
				}
				group.Wait()
				close(errors)
				for err := range errors {
					require.NoError(t, err)
				}
				require.NoError(t, db.First(&user, user.Id).Error)
				assert.Equal(t, 3665000, user.Quota)
			})

			t.Run("cross-cutover usage totals retain separate units and filters", func(t *testing.T) {
				operation_setting.GetPointsMigrationSetting().CutoverTime = cutover
				logs := []model.Log{
					{Type: model.LogTypeConsume, Username: "points_stats", ModelName: "gemini-test", CreatedAt: cutover - 1, Quota: 500000},
					{Type: model.LogTypeConsume, Username: "points_stats", ModelName: "gemini-test", CreatedAt: cutover, Quota: 1000000},
					{Type: model.LogTypeConsume, Username: "other_user", ModelName: "gemini-test", CreatedAt: cutover - 1, Quota: 9000000},
				}
				require.NoError(t, db.Create(&logs).Error)
				stat, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "gemini-test", "points_stats", "", 0, "")
				require.NoError(t, err)
				assert.Equal(t, 1500000, stat.Quota)
				assert.Equal(t, 500000, stat.LegacyQuota)
				stat, err = model.SumUsedQuota(model.LogTypeConsume, cutover, 0, "gemini-test", "points_stats", "", 0, "")
				require.NoError(t, err)
				assert.Equal(t, 1000000, stat.Quota)
				assert.Zero(t, stat.LegacyQuota)
			})
		})
	}
}
