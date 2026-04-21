const { verifyToken } = require('../utils/jwt');
const { User } = require('../models');
const logger = require('../utils/logger');

const authMiddleware = async (req, res, next) => {
  try {
    // 从请求头中获取Authorization token
    const authHeader = req.headers.authorization;
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      logger.warn('Authentication failed: No authorization token provided');
      return res.status(401).json({ error: 'Authorization token required' });
    }

    // 提取token
    const token = authHeader.split(' ')[1];

    // 验证token
    const decoded = verifyToken(token);
    if (!decoded) {
      logger.warn('Authentication failed: Invalid or expired token');
      return res.status(401).json({ error: 'Invalid or expired token' });
    }

    // 查找用户
    const user = await User.findByPk(decoded.id);
    if (!user) {
      logger.warn(`Authentication failed: User not found for ID ${decoded.id}`);
      return res.status(401).json({ error: 'User not found' });
    }

    // 记录认证成功
    logger.info(`User authenticated successfully: ${user.username} (ID: ${user.id})`);

    // 将用户信息添加到请求对象中
    req.user = {
      id: user.id,
      username: user.username,
      email: user.email
    };

    next();
  } catch (error) {
    logger.error(`Error in auth middleware: ${error.message}`);
    res.status(401).json({ error: 'Authentication failed' });
  }
};

module.exports = authMiddleware;