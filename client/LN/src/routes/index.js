const express = require('express');
const router = express.Router();
const authRoutes = require('./auth');
const protectedRoutes = require('./protected');

// 认证路由
router.use('/auth', authRoutes);

// 受保护的路由
router.use('/api', protectedRoutes);

module.exports = router;