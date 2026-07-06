# Contest Config

A contest is a **database record** created via `POST /api/v1/admin/contests` and managed through the admin API. The contest definition, its ordered problem list, its announcements, and its static assets are all stored in the database — **not** on disk. There is no `contest.yaml` file and no per-contest directory.

## JSON Shape

```json
{
  "id": "sample-contest-1",
  "name": "Sample Introductory Contest",
  "starttime": "2025-10-01T09:00:00+08:00",
  "endtime": "2025-10-01T12:00:00+08:00",
  "problems": ["p1001-aplusb", "p1002-fizzbuzz"],
  "description": "# Sample Introductory Contest\n\nWelcome..."
}
```

## Creating a Contest

```bash
curl -X POST /api/v1/admin/contests \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "sample-contest-1",
    "name": "Sample Introductory Contest",
    "starttime": "2025-10-01T09:00:00+08:00",
    "endtime": "2025-10-01T12:00:00+08:00",
    "description": "# Sample Introductory Contest"
  }'
```

## Managing a Contest's Problems

- **Add a problem** — `POST /api/v1/admin/contests/:id/problems` creates the problem record and appends its ID to the contest's ordered `problems` list.
- **Reorder problems** — `PUT /api/v1/admin/contests/:id/problems/order` with body `{ "problem_ids": ["p1002-fizzbuzz", "p1001-aplusb"] }` sets the full ordered list. The request must contain the same set of problem IDs as the current list (just reordered); duplicates or foreign IDs are rejected.

The `problems` field on the contest record is the source of truth for ordering; individual problem definitions live in their own records (see [Problem Config](./problem-config.md)).

## Updating and Deleting

- **Update** — `PUT /api/v1/admin/contests/:id` with the full Contest JSON. The `problems` list is preserved (it is managed via the problem endpoints above).
- **Delete** — `DELETE /api/v1/admin/contests/:id` removes the contest record. (Problem records belonging to the contest should be deleted individually via `DELETE /api/v1/admin/problems/:id`.)

All write endpoints trigger an in-memory `reload` so the running server picks up the change immediately.

-----

## Field Reference

### `id`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: A globally unique identifier for the contest. It's recommended to use an easily recognizable string, like `final-2025`.

-----

### `name`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: The display name of the contest.

-----

### `starttime`

  - **Type**: `string` (ISO 8601 format)
  - **Required**: Yes
  - **Description**: The official start time of the contest. Before this time, users cannot view the problem list or submit code.
  - **Format**: `YYYY-MM-DDTHH:MM:SSZ` or `YYYY-MM-DDTHH:MM:SS±hh:mm`. For example, `2025-10-26T14:00:00+08:00` represents 2 PM Beijing time.

-----

### `endtime`

  - **Type**: `string` (ISO 8601 format)
  - **Required**: Yes
  - **Description**: The end time of the contest. After this time, users cannot submit code.

-----

### `problems`

  - **Type**: `array of strings`
  - **Required**: No (managed via the problem endpoints)
  - **Description**: The ordered list of problem IDs included in the contest. Order is managed via `PUT /api/v1/admin/contests/:id/problems/order`; appending a problem via `POST /api/v1/admin/contests/:id/problems` adds its ID to the end of this list.

-----

### `description`

  - **Type**: `string` (Markdown)
  - **Required**: No
  - **Description**: The contest description shown on the frontend, written in Markdown. Static assets referenced from the description are uploaded and managed via the contest asset endpoints (`POST /api/v1/admin/contests/:id/assets`).
