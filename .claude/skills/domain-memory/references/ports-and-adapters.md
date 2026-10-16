# Ports and adapters

A Port is a conversation the domain needs to have, stated in the domain's language and owned by the Context that needs it. An Adapter is the one place that knows who is on the other side. This page decides whether a change needs that boundary and proves that it holds. It prescribes no folder, file name, or language construct; read the neighbouring code for those.

## When a Port is earned

A Port is earned by what is on the other side, not by the pattern's name:

- something outside the process that can be replaced, fail, answer late, answer twice, or cost money: a provider, a store, a queue, a clock;
- a reviewed force saying the collaborator is expected to change;
- a proof obligation that needs the test to decide what the outside answers.

It is not earned by a helper inside the same Context, by a class that has one implementation and nothing external behind it, or by a wish to give every class an interface. Expect few Ports: one for each purposeful conversation, not one for each operation.
## Questions before coding

1. What does the domain want from this conversation, in its own terms? The answer names the Port. A name that mentions the provider, the protocol, or the transport is the Adapter's name.
2. Who starts the conversation: the outside asking the application to act, or the application asking the outside?
3. Which outcomes must the domain tell apart? Those are the Port's vocabulary. A provider's status codes and error classes are not.
4. What must stay true when the other side fails, is slow, or answers twice? Take the answer from the handoff's forces and proof obligations.
5. If the provider were replaced tomorrow, which files would change? A file that holds a domain decision must not be on that list.

## What each side holds

| Side | Holds |
| --- | --- |
| Port | The domain's request and the outcomes the domain distinguishes, in domain types. |
| Adapter | Address, protocol, credentials, configuration keys, provider status vocabulary, transport-level retry, and the translation of a provider failure into a domain outcome. |
| Assembly point | The one place that chooses which Adapter stands behind a Port. A domain decision does not construct its own Adapter. |

A file that holds a domain decision imports the Port and nothing that names the provider. A Port declared in the Adapter's file fails this even when the decision never mentions the provider, because reaching the Port loads the provider with it.

A failure the domain survives is still an outcome. The Adapter translates it, and the caller hands it to whatever records it or acts on it. Catching everything and carrying on keeps the reviewed fact and hides the failure from whoever has to repair it.

Two Contexts do not share a provider's types as a common language. Each states what it needs in its own terms, and the assembly point translates.

## At the assembly point

- Configuration that changes how an immutable record behaves is captured in that record when it is made, not read again on a retry. Capture every value that decides compatibility or safety, including a limitation someone explicitly accepted.
- Clients, transports and timeouts are chosen at the entry point and handed to the Adapter. A focused test proves the chosen client is the one that reaches it.
- The Adapter owns its request and response contract; the assembly point owns the endpoint, the credentials and the client.
- When the caller passes a deadline, the assembly point adds no shorter timeout of its own: one place decides how long to wait.
- The entry point reads the environment once and passes typed values on. Setting types and their validation may be shared; reading the environment is not, and a helper that falls back between values chooses only among values it is given.
- Tests build Adapters through the same assembly as production. A fallback that only tests rely on does not replace the entry point supplying the real client.
- Persistence that the data-access tooling cannot express is confined to one Adapter at the entry point, and any record of who owns that data points at that Adapter.

## Evidence

| Claim | Evidence |
| --- | --- |
| The provider stays behind its Adapter | The repository's dependency rule, or a search when it has none, shows the provider's names only in the Adapter and the assembly point. |
| The stand-in tells the truth | The same expectations run against the stand-in and the real Adapter. |
| A failure preserves the reviewed fact | A test makes the stand-in fail and observes the state the handoff promises. |
| The Port is earned | Removing it would force a file that holds a domain decision to name the provider. |

Report what was not run. An Adapter that was never exercised against its real counterpart is unverified, and the report says so instead of letting a passing stand-in speak for it.

## Misuses to refuse

- A business rule placed in the Adapter because the data happened to be there.
- A Port that repeats the provider's operations one for one.
- A domain type that carries a wire format's field names or a framework's annotations.
- An interface that [is not earned](#when-a-port-is-earned).
