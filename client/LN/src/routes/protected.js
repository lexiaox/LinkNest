const express = require('express');
const router = express.Router();
const authMiddleware = require('../middleware/auth');

// 受保护的路由，需要认证
router.get('/profile', authMiddleware, (req, res) => {
  res.status(200).json({
    message: 'Protected route accessed successfully',
    user: req.user
  });
});

module.exports = router;