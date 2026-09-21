-- 微信第三方平台管理平台：数据库初始化（建库 + 建用户 + 授权）
-- 用法：mysql -u root < db/init.sql
-- 注意：生产环境请修改密码，并与 server/.env 中的 DB_PASSWORD 保持一致。

CREATE DATABASE IF NOT EXISTS wx_platform CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER IF NOT EXISTS 'wxplatform'@'localhost' IDENTIFIED BY 'wxplatform_dev_password';
CREATE USER IF NOT EXISTS 'wxplatform'@'127.0.0.1' IDENTIFIED BY 'wxplatform_dev_password';

GRANT ALL PRIVILEGES ON wx_platform.* TO 'wxplatform'@'localhost';
GRANT ALL PRIVILEGES ON wx_platform.* TO 'wxplatform'@'127.0.0.1';
FLUSH PRIVILEGES;

-- 集成测试库（可选；设置了 TEST_DB_DSN 时各包的集成测试才会执行）
CREATE DATABASE IF NOT EXISTS wx_platform_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON wx_platform_test.* TO 'wxplatform'@'localhost';
GRANT ALL PRIVILEGES ON wx_platform_test.* TO 'wxplatform'@'127.0.0.1';

-- 端到端测试专用库：e2e 会清空全部业务表，单独一个库可避免与各包的集成测试互相干扰
CREATE DATABASE IF NOT EXISTS wx_platform_e2e CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON wx_platform_e2e.* TO 'wxplatform'@'localhost';
GRANT ALL PRIVILEGES ON wx_platform_e2e.* TO 'wxplatform'@'127.0.0.1';
FLUSH PRIVILEGES;
