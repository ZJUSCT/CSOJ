# Contest Config (contest.yaml)

Each contest is defined by a separate directory, the path of which must be declared in the `contest` list in the main `config.yaml` file.

A contest directory must contain a `contest.yaml` file and may optionally include an `index.md` file for the contest description.

## Directory Structure Example

```

contests/sample-contest/
├── contest.yaml      \# The core configuration file for the contest
├── index.md          \# (Optional) Detailed contest description in Markdown
└── p1001-aplusb/     \# A problem directory; the name is arbitrary

```

---

## `contest.yaml` Example

```yaml
# The unique ID for the contest
id: "sample-contest-1"

# The name of the contest to be displayed on the frontend
name: "Sample Introductory Contest"

# Contest start time (ISO 8601 format)
starttime: "2025-10-01T09:00:00+08:00"

# Contest end time (ISO 8601 format)
endtime: "2025-10-01T12:00:00+08:00"

# A list of problems included in the contest
# Each item is a relative path to a directory containing a problem.yaml file
problems:
  - "p1001-aplusb"
  - "p1002-fizzbuzz"

```

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
  - **Required**: Yes
  - **Description**: Defines which problems are included in the contest. Each string in the array is a directory path **relative to the current `contest.yaml` file**. This directory must contain a `problem.yaml` file.
