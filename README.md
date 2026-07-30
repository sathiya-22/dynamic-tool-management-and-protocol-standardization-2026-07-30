# AgentTool Registry & Protocol (ATR&P)

## The Real Problem and Who It Affects
The agentic AI community currently struggles with dynamic tool management and a lack of standardized protocols for tool calling. In complex multi-agent systems, developers need robust mechanisms to add, remove, and update tools at runtime without system restarts. Furthermore, the absence of a common protocol for programmatic tool invocation leads to significant boilerplate, tight coupling, and poor interoperability between agents and tools developed by different teams or using different frameworks. This affects anyone building multi-agent systems, tool developers, and framework architects who want to enable flexible and scalable agentic applications.

## Why This Project Shape/Stack Was Chosen
This project is a **Go service** implementing a central **AgentTool Registry & Protocol (ATR&P)**.
- **Go** was chosen for its excellent concurrency primitives, strong typing, and performance, making it ideal for a high-throughput, low-latency service that manages tool registrations and dispatches tool calls.
- A **service** architecture provides a clear separation of concerns, allowing agents (potentially written in any language) to interact with a centralized tool management system via a well-defined API. This solves the dynamic management problem by centralizing tool lifecycle.
- It directly addresses the "standardized protocols" demand by defining a simple HTTP/JSON API for registration and execution, serving as a concrete, provider-agnostic protocol reference. This avoids language-specific SDKs for the core protocol.

## Setup and Usage Instructions (Zero API Keys Required)

This prototype runs entirely locally and requires no external API keys.

### Prerequisites
- Go (version 1.18 or higher)

### Setup
1. Clone this repository:
   ```bash
   git clone <repository-url>
   cd <repository-directory>
   ```

2. Run the server:
   ```bash
   go run main.go
   ```
   The server will start on `http://localhost:8080`.

### Usage (with `curl`)

#### 1. Register a Tool
Let's register a simple "calculator" tool and a "weather" tool.

**Calculator Tool:**
```bash
curl -X POST -H "Content-Type: application/json" -d '{
    "name": "calculator",
    "description": "Performs basic arithmetic operations.",
    "endpoint": "http://localhost:8081/calculate",
    "schema": {
        "type": "object",
        "properties": {
            "operation": {"type": "string", "enum": ["add", "subtract", "multiply", "divide"]},
            "a": {"type": "number"},
            "b": {"type": "number"}
        },
        "required": ["operation", "a", "b"]
    }
}' http://localhost:8080/tools
```

**Weather Tool:**
```bash
curl -X POST -H "Content-Type: application/json" -d '{
    "name": "weather_forecast",
    "description": "Fetches the current weather for a given city.",
    "endpoint": "http://localhost:8082/weather",
    "schema": {
        "type": "object",
        "properties": {
            "city": {"type": "string", "description": "The city name for weather forecast"}
        },
        "required": ["city"]
    }
}' http://localhost:8080/tools
```
*(Note: These `endpoint` URLs are illustrative. For a real system, you'd have microservices implementing these tools. For this prototype, the ATR&P simply forwards the payload.)*

#### 2. List Registered Tools
```bash
curl http://localhost:8080/tools
```
You should see both `calculator` and `weather_forecast` in the response.

#### 3. Call a Tool
Let's simulate an agent calling the `calculator` tool. The ATR&P acts as a proxy, forwarding the call to the registered `endpoint`.

```bash
curl -X POST -H "Content-Type: application/json" -d '{
    "tool_name": "calculator",
    "arguments": {
        "operation": "add",
        "a": 5,
        "b": 3
    }
}' http://localhost:8080/execute
```
The ATR&P will validate the arguments against the registered schema and, if valid, forward the call to `http://localhost:8081/calculate` (which, in a real setup, would be your calculator microservice). For this prototype, it will return a success message indicating the forwarding.

#### 4. Remove a Tool
```bash
curl -X DELETE http://localhost:8080/tools/calculator
```

#### 5. Verify Removal
```bash
curl http://localhost:8080/tools
```
The `calculator` tool should no longer be listed.

## LLM Integration
This project is pure orchestration/protocol work with no direct LLM involvement. The ATR&P provides the *infrastructure* for managing and calling tools. Agents, which might be powered by LLMs, would interact with this service to discover and invoke tools. The tool definitions and execution payloads are designed to be compatible with typical LLM "function calling" or "tool use" output formats, but the ATR&P itself does not process LLM input/output or integrate with any LLM SDKs. No optional real-LLM adapter exists because it's outside the scope of this infrastructure piece.
