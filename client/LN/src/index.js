const express = require('express');
const cors = require('cors');
const routes = require('./routes');
const { syncDatabase } = require('./models');

const app = express();
const PORT = process.env.PORT || 3000;

// 中间件
app.use(cors());
app.use(express.json());

// 路由
app.use('/api', routes);

// 健康检查
app.get('/health', (req, res) => {
  res.status(200).json({ status: 'ok' });
});

// 启动服务器
app.listen(PORT, async () => {
  console.log(`Server running on port ${PORT}`);
  // 同步数据库
  await syncDatabase();
});