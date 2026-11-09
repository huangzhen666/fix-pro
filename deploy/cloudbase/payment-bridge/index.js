// CloudBase HTTP functions use a listening process, rather than an event handler.
const { createServer } = require('./index.cjs')
createServer().listen(Number(process.env.PORT || 9000), '0.0.0.0')
