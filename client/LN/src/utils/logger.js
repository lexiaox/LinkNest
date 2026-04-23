const fs = require('fs');
const path = require('path');

// 确保日志目录存在
const logDir = path.join(__dirname, '../../logs');
if (!fs.existsSync(logDir)) {
  fs.mkdirSync(logDir, { recursive: true });
}

// 日志文件名
const logFileName = path.join(logDir, `${new Date().toISOString().split('T')[0]}.log`);

const logger = {
  // 日志级别
  levels: {
    error: 0,
    warn: 1,
    info: 2,
    debug: 3
  },
  
  // 当前日志级别
  level: 'info',
  
  // 记录日志
  log(level, message) {
    if (this.levels[level] <= this.levels[this.level]) {
      const timestamp = new Date().toISOString();
      const logMessage = `[${timestamp}] [${level.toUpperCase()}] ${message}\n`;
      
      // 输出到控制台
      console.log(logMessage.trim());
      
      // 写入文件
      try {
        fs.appendFileSync(logFileName, logMessage);
      } catch (error) {
        console.error('Error writing to log file:', error);
      }
    }
  },
  
  // 错误日志
  error(message) {
    this.log('error', message);
  },
  
  // 警告日志
  warn(message) {
    this.log('warn', message);
  },
  
  // 信息日志
  info(message) {
    this.log('info', message);
  },
  
  // 调试日志
  debug(message) {
    this.log('debug', message);
  }
};

module.exports = logger;