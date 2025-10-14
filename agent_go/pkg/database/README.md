# Chat History Database

This package provides a simple, typed interface for storing chat history using SQLite as the default database.

## Features

- **Interface-based Design**: Easy to switch between different database providers
- **SQLite Support**: Lightweight, file-based database for development and small deployments
- **PostgreSQL Support**: Production-ready database support
- **Typed Events**: Stores all 67 event types from the unified events system
- **Simple API**: Clean, RESTful API for chat history management

## Database Schema

### Tables

1. **chat_sessions**: Stores chat session metadata
2. **events**: Stores all typed events as JSON
3. **conversation_summaries**: Stores conversation turn summaries

### Key Features

- **String IDs**: Uses string IDs instead of UUIDs for simplicity
- **JSON Storage**: Events stored as JSON for flexibility
- **Foreign Keys**: Proper relationships between tables
- **Indexes**: Optimized for common queries

## API Endpoints

### Chat Sessions
- `POST /api/chat-history/sessions` - Create a new chat session
- `GET /api/chat-history/sessions` - List all chat sessions (with pagination)
- `GET /api/chat-history/sessions/{session_id}` - Get specific session
- `PUT /api/chat-history/sessions/{session_id}` - Update session
- `DELETE /api/chat-history/sessions/{session_id}` - Delete session

### Events
- `GET /api/chat-history/sessions/{session_id}/events` - Get events for a session
- `GET /api/chat-history/events` - Search events with filters

### Conversation Data
- `GET /api/chat-history/sessions/{session_id}/summaries` - Get conversation summaries
- `GET /api/chat-history/sessions/{session_id}/details` - Get full conversation details
- `GET /api/chat-history/sessions/{session_id}/flow` - Get conversation flow

### Health
- `GET /api/chat-history/health` - Health check

## Usage

### Starting the Server

```bash
# Start with default SQLite database
go run main.go server

# Start with custom database path
go run main.go server --db-path /path/to/chat.db

# Start with PostgreSQL
go run main.go server --db-path "postgres://user:pass@localhost/dbname"
```

### Testing

```bash
# Run the test script
./test_chat_history.sh
```

## Database Providers

### SQLite (Default)
- **File-based**: No server required
- **Perfect for**: Development, testing, small deployments
- **Driver**: `github.com/mattn/go-sqlite3`

### PostgreSQL
- **Server-based**: Requires PostgreSQL server
- **Perfect for**: Production, high-volume deployments
- **Driver**: `github.com/lib/pq`

## Event Integration

The database automatically integrates with the existing event system:

1. **Event Storage**: All events are automatically stored when emitted
2. **Typed Events**: Supports all 67 event types from the unified system
3. **Session Tracking**: Events are linked to chat sessions
4. **Real-time**: Events are stored as they happen

## Future Enhancements

- **MongoDB Support**: Document-based storage for events
- **Redis Caching**: Fast access to recent events
- **Event Compression**: Reduce storage for large event payloads
- **Analytics**: Built-in analytics and reporting
- **Backup/Restore**: Database backup and restore functionality
