# agentd

A distributed runtime for AI agents. You deploy agents to a cluster the same way you deploy containers — the platform handles scheduling, scaling, and recovery. You write the logic, agentd handles the rest.

The interesting part: agents don't just run tasks. They argue with each other. Every prompt goes to multiple models simultaneously. They propose answers, challenge each other's reasoning, and an arbitrator resolves the disagreement. The dissents get stored. Over time you accumulate a dataset of exactly where different models diverge — which turns out to be more useful than the answers themselves.

---

## what it does

- runs agents as scheduled workloads across a cluster of worker nodes
- routes tasks through NATS so agents communicate without knowing about each other
- isolates per-agent memory in Redis with namespace scoping
- scales workers up and down based on queue depth
- runs multi-model negotiation rounds: propose → challenge → resolve
- persists every dissent to Redis so you can query disagreement patterns over time
- exposes everything over gRPC with a CLI that talks to it

---

## quick start

you need Go 1.21+, NATS server, Redis, and Ollama running locally.

```bash
# install dependencies
sudo pacman -S redis
nats-server &
ollama pull llama3.2
ollama pull mistral

# clone and build
git clone https://github.com/AnantSingh1510/agentd
cd agentd
go build -o agentd ./cmd/agentd

# start the server
./agentd serve

# in another terminal — run a negotiation round
./agentd negotiate "Should AI development be paused until safety is solved"
```

---

## how a negotiation round works

three participants, one prompt:

- **agent-alpha** (llama3.2) argues FOR the position
- **agent-beta** (mistral) argues AGAINST it
- **agent-arb** (llama3.2) reads both arguments and the cross-challenges, then picks a winner

```
prompt ──► llama3.2 (FOR)  ──► proposal A ──┐
       └─► mistral  (AGAINST) ──► proposal B ──┤──► challenges ──► arbitration ──► verdict
                                               │                        │
                                               └────────────────────────┘
                                                    dissents stored to Redis
```

the dissents are the point. when the models disagree, you get a structured record of what each argued and why the arbitrator sided with one over the other. run enough rounds and patterns emerge.

---

## cli

```bash
# start the server (boots autoscaler + negotiator)
./agentd serve

# live dashboard — workers, metrics, divergence feed
./agentd top

# submit a raw task to the worker pool
./agentd submit "summarize the CAP theorem"

# run a negotiation round
./agentd negotiate "is consistency or availability more important"

# query the dissent dataset
./agentd divergence list
./agentd divergence stats

# point at a different server
./agentd --server my-cluster:50051 negotiate "..."
```

---

## architecture

```
cmd/agentd                   CLI — serve, top, submit, negotiate, divergence
controlplane/
  api/                       gRPC server (submit tasks, list workers, run negotiations)
  autoscaler/                watches queue depth, spawns/removes workers
negotiation/                 propose → challenge → resolve protocol
worker/                      executes tasks, emits completed/failed events
adapters/ollama/             LLM adapter — swap for any backend
divergence/                  persists dissents to Redis, query by model or round
kernel/
  scheduler/                 assigns tasks to workers, queues on backpressure
  broker/                    NATS pub/sub wrapper
  memory/                    Redis namespaced key-value per agent
  types/                     shared types (no deps, no cycles)
```

the kernel is intentionally minimal. it does three things: scheduling, message passing, and memory isolation. everything else is outside it. if you want to add a new LLM adapter or change the negotiation protocol, you never touch the kernel.

---

## adding a model

implement the `ModelHandler` interface and wire it into a participant:

```go
type ModelHandler func(ctx context.Context, prompt []byte) ([]byte, string, error)
//                                                          answer  reasoning
```

the ollama adapter is about 60 lines. adding OpenAI or Anthropic is the same shape.

```go
// adapters/ollama/ollama.go — the whole thing
func (c *Client) Handler() negotiation.ModelHandler {
    return func(ctx context.Context, prompt []byte) ([]byte, string, error) {
        response, err := c.Generate(ctx, string(prompt))
        return []byte(response), fmt.Sprintf("via %s", c.model), err
    }
}
```

---

## the divergence dataset

every negotiation round where models disagree produces a `Dissent` record:

```go
type Record struct {
    RoundID   string
    TaskID    string
    Prompt    string
    Dissent   negotiation.Dissent  // AgentID, ModelName, Position
    Verdict   string
    CreatedAt time.Time
}
```

stored in Redis under `dissent:<roundID>:<agentID>`. query it:

```bash
./agentd divergence list     # all dissents, most recent first
./agentd divergence stats    # dissent count per model
```

```
MODEL                 DISSENTS
────────────────────  ────────
llama3.2              4
mistral               3
```

this is the part that gets interesting at scale. you start seeing which model dissents more on political questions vs technical ones, where they reliably agree, and where the arbitrator consistently sides with one over the other.

---

## running tests

```bash
# unit tests (no external deps)
go test -short ./...

# full suite (requires NATS + Redis running)
go test ./...

# specific package
go test ./negotiation/... -v
```

58 tests across 9 packages.

---

## project status

the kernel, worker, negotiation protocol, autoscaler, gRPC API, divergence store, and CLI are all built and tested. what's missing:

- streaming negotiation events (dashboard shows final state, not in-progress)
- OpenAI and Anthropic adapters (only Ollama right now)
- persistence for negotiation rounds beyond Redis TTL
- auth on the gRPC server
- multi-node deployment (currently single-machine)

pull requests welcome on any of these. the kernel is intentionally frozen — new functionality goes in adapters, the negotiation layer, or the control plane.

---

## why

agent frameworks today are libraries. they help you write agent logic but leave the operational problem to you. if you want ten agents running in parallel, recovering from failure, sharing context, and talking to each other — you're on your own.

agentd is an attempt at the infrastructure layer that's missing. the negotiation protocol is the piece that doesn't exist anywhere else: treating model disagreement as a first-class signal rather than something to paper over.

---

built with Go, NATS, Redis, Ollama, bubbletea, and grpc.