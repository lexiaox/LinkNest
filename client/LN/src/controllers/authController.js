const { User } = require('../models');
const { hashPassword, comparePassword } = require('../utils/password');
const jwt = require('jsonwebtoken');
const logger = require('../utils/logger');
require('dotenv').config();

const register = async (req, res) => {
  try {
    const { username, email, password } = req.body;
    
    // 记录注册请求
    logger.info(`Register request received for username: ${username}, email: ${email}`);

    // 验证请求数据
    if (!username || !password) {
      logger.warn(`Registration failed for ${username}: Missing required fields`);
      return res.status(400).json({ error: 'Username and password are required' });
    }

    // 检查用户名是否已存在
    const existingUser = await User.findOne({ where: { username } });
    if (existingUser) {
      logger.warn(`Registration failed for ${username}: Username already exists`);
      return res.status(400).json({ error: 'Username already exists' });
    }

    // 检查邮箱是否已存在（如果提供了邮箱）
    if (email) {
      const existingEmail = await User.findOne({ where: { email } });
      if (existingEmail) {
        logger.warn(`Registration failed for ${username}: Email already exists`);
        return res.status(400).json({ error: 'Email already exists' });
      }
    }

    // 哈希密码
    const hashedPassword = await hashPassword(password);

    // 创建用户
    const user = await User.create({
      username,
      email,
      password_hash: hashedPassword
    });

    // 记录注册成功
    logger.info(`User registered successfully: ${username} (ID: ${user.id})`);

    // 返回用户信息（不包含密码）
    res.status(201).json({
      id: user.id,
      username: user.username,
      email: user.email
    });
  } catch (error) {
    logger.error(`Error registering user: ${error.message}`);
    res.status(500).json({ error: 'Internal server error' });
  }
};

const login = async (req, res) => {
  try {
    const { username, password } = req.body;
    
    // 记录登录请求
    logger.info(`Login request received for username: ${username}`);

    // 验证请求数据
    if (!username || !password) {
      logger.warn(`Login failed for ${username}: Missing required fields`);
      return res.status(400).json({ error: 'Username and password are required' });
    }

    // 查找用户
    const user = await User.findOne({ where: { username } });
    if (!user) {
      // 统一错误信息，不区分用户不存在和密码错误
      logger.warn(`Login failed for ${username}: Invalid username or password`);
      return res.status(401).json({ error: 'Invalid username or password' });
    }

    // 验证密码
    const isPasswordValid = await comparePassword(password, user.password_hash);
    if (!isPasswordValid) {
      // 统一错误信息，不区分用户不存在和密码错误
      logger.warn(`Login failed for ${username}: Invalid username or password`);
      return res.status(401).json({ error: 'Invalid username or password' });
    }

    // 生成JWT token
    const token = jwt.sign(
      { id: user.id, username: user.username },
      process.env.JWT_SECRET,
      { expiresIn: process.env.JWT_EXPIRES_IN }
    );

    // 记录登录成功
    logger.info(`User logged in successfully: ${username} (ID: ${user.id})`);

    // 返回token和用户信息
    res.status(200).json({
      token,
      user: {
        id: user.id,
        username: user.username,
        email: user.email
      }
    });
  } catch (error) {
    logger.error(`Error logging in: ${error.message}`);
    res.status(500).json({ error: 'Internal server error' });
  }
};

module.exports = {
  register,
  login
};