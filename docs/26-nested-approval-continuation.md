# 26 — Approval pauses a cell, not the whole program forever

A nested tool can require approval just like a direct tool. Its result belongs
to the cell, so a direct-tool approval resume is incorrect: that would record
the result in agent history and lose the program's continuation.

The teaching runtime makes the pause explicit:

```text
CellProgram calls nested tool
  → registry returns ApprovalRequest
  → program returns CellApproval { request, resume continuation }
  → cell state = waiting_approval
  → Session exposes the request to the UI

user approves
  → execute exactly the original nested call
  → send ToolResult to continuation
  → cell yields/completes one CellOutput
  → outer turn loop resumes
```

The continuation is a Go closure only because this is an in-process teaching
runtime. A real JavaScript runtime keeps the suspended stack and promise state
inside the cell. The invariant is the same: approval resumes the suspended
operation; it does not restart the program from the beginning.
