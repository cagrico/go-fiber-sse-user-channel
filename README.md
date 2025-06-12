# go-fiber-sse-user-channel

A clean and minimal Go Fiber v3 project that demonstrates how to use **Server-Sent Events (SSE)** to send real-time messages to clients based on user IDs.

This project is ideal as a reusable module or reference for integrating user-targeted SSE messaging in your own applications.

---

## 🚀 Features

* 🔄 Server-Sent Events (SSE) over HTTP
* 👤 Multiple sessions per user (`userID`)
* 📡 Broadcast messages to all sessions of a given user
* ✅ Graceful shutdown support
* 📊 System and runtime monitoring (`/metrics/system` endpoint)
* ⚙️ Built with **Go Fiber v3**

---

## 📦 Requirements

* Go 1.20+
* [Fiber v3](https://github.com/gofiber/fiber)
* [gopsutil](https://github.com/shirou/gopsutil) (for system metrics)

Install dependencies:

```bash
go get github.com/gofiber/fiber/v3
go get github.com/shirou/gopsutil/v3
```

---

## 🛠 Installation

```bash
git clone https://github.com/your-username/go-fiber-sse-user-channel.git
cd go-fiber-sse-user-channel
go run main.go
```

> Server will run on `http://localhost:8080`

---

## 📘 API Endpoints

### 1. `GET /sse?userID=123`

Establishes an SSE connection for the given user ID.

**Example cURL:**

```bash
curl -N http://localhost:8080/sse?userID=123
```

---

### 2. `POST /send-to-user`

Broadcasts a message to all active sessions of a given user.

**Request Body:**

```json
{
  "userID": "123",
  "value": {
    "message": "Hello world!"
  }
}
```

**Response:**

```json
{
  "sent": 2
}
```

---

### 3. `GET /health`

Basic health check endpoint.

---

### 4. `GET /connections`

Returns the number of open HTTP connections and active sessions.

---

### 5. `GET /metrics/system`

Returns detailed system and Go runtime metrics including:

* CPU usage %
* RAM usage (used / total)
* Go memory stats
* GC cycles
* Active goroutines
* Timestamp

Useful for observability and debugging.

---

### 🧪 Example Client (HTML)

You can test the SSE functionality using the included example HTML file:

```bash
open examples/sse-client.html
```

Make sure the Go server is running on `http://localhost:8080`, then open the file in your browser and enter a user ID to start receiving real-time messages.

---

## 🧼 Graceful Shutdown

When you press `Ctrl+C` or terminate the process:

* All active SSE connections are closed
* Channels are cleaned up
* The server exits cleanly within a 5-second timeout

---

## 💡 Use Cases

* Real-time user notifications (e.g. order status updates)
* One-way messaging without full WebSocket complexity
* Live dashboards or system updates per user

---

## 🤝 Contributing

Feel free to fork this repo or open an issue / PR if you have suggestions or improvements.

---

## 📄 License

MIT — see [LICENSE](./LICENSE) for details.
