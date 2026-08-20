# kumctl API E2E Report

- Started: `2026-08-20T06:25:48Z`
- API: `http://127.0.0.1:31080`
- Environment: MySQL-backed Demo + Kind Hub (`kind-kumquat-hub`)
- Executed cases: **41** (37 included API operations + 3 explicit policy checks + 1 cleanup assertion)
- Passed: **41**
- Failed: **0**

## Coverage

| API group | Operations |
|---|---:|
| auth (login) | 1 |
| users | 5 |
| roles | 3 |
| modules | 6 |
| projects | 6 |
| workspaces | 6 |
| applications | 6 |
| clusters | 3 |
| operations | 1 |
| **Included API operations** | **37** |
| Explicitly excluded auth operations | register, me, change-password |

## Case Results

| Result | Case |
|---|---|
| PASS | `POST /auth/login (kumctl login)` |
| PASS | `GET /roles` |
| PASS | `GET /roles/{id}` |
| PASS | `GET /roles/{id}/permissions` |
| PASS | `GET /users` |
| PASS | `POST /users` |
| PASS | `GET /users/{id}` |
| PASS | `PUT /users/{id}` |
| PASS | `DELETE /users/{id}` |
| PASS | `GET /modules` |
| PASS | `POST /modules` |
| PASS | `GET /modules/{id}` |
| PASS | `PUT /modules/{id}` |
| PASS | `GET /modules/{id}/children` |
| PASS | `GET /projects` |
| PASS | `POST /projects` |
| PASS | `GET /projects/{id}` |
| PASS | `PUT /projects/{id}` |
| PASS | `GET /projects/module/{moduleId}` |
| PASS | `GET /workspaces` |
| PASS | `POST /workspaces` |
| PASS | `GET /operations/{id}` |
| PASS | `GET /workspaces/{id}` |
| PASS | `PUT /workspaces/{id}` |
| PASS | `GET /applications` |
| PASS | `POST /applications` |
| PASS | `GET /applications/{id}` |
| PASS | `PUT /applications/{id}` |
| PASS | `POST /workspaces/{id}/adopt-existing` |
| PASS | `POST /applications/{id}/adopt-existing` |
| PASS | `GET /clusters` |
| PASS | `GET /clusters/{id}` |
| PASS | `POST /clusters/{id}/adopt` |
| PASS | `DELETE /applications/{id}` |
| PASS | `DELETE /workspaces/{id}` |
| PASS | `DELETE /projects/{id}` |
| PASS | `DELETE /modules/{id}` |
| PASS | `DELETE /modules/{id} (root cleanup)` |
| PASS | `excluded auth/register` |
| PASS | `excluded auth/me` |
| PASS | `excluded auth/change-password` |

## Method

Each case invokes the compiled kumctl binary in a separate process. Requests traverse the API gateway, asynchronous mutations are checked through /operations/{id}, and no included operation is skipped.

## Effects verified

- Login token was accepted by every subsequent authenticated request.
- CRUD mutations were followed by independent get/list checks and fixture cleanup.
- Workspace/application create, update, adopt, and delete operations reached a successful terminal operation state.
- A real Engine Edge ManagedCluster was discovered, read, and adopted through the API gateway.
- The excluded auth/register, auth/me, and auth/change-password commands were explicitly rejected by kumctl policy.
