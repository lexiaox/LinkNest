const logger = require('./src/utils/logger');

// 测试不同级别的日志
logger.error('This is an error message');
logger.warn('This is a warning message');
logger.info('This is an info message');
logger.debug('This is a debug message');

console.log('Logger test completed. Check the logs directory for log files.');