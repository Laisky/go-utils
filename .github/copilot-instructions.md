## General

### TimeZone

Always use UTC for time handling in servers, databases, and APIs.

### Testing

Please create suitable unit tests based on the current project circumstances. Whenever a new issue arises, update the unit tests during the fix to ensure thorough coverage of the problem by the test cases. Avoid creating temporary, one-off test scripts, and focus on continuously enhancing the unit test cases.

## Golang Style

### Context

Whenever feasible, utilize context to manage the lifecycle of the call chain.

### Golang Error Handling

All errors should be handled, and the error handling should be as close to the source of the error as possible.

Never use `err == nil` to avoid shadowing the error variable.

Use `github.com/Laisky/errors/v2`, its interface is as same as `github.com/pkg/errors`. Never return bare error, always wrap it by `errors.Wrap`/`errors.Wrapf`/`errors.WithStack`, check all files

### Golang ORM

Use `gorm.io/gorm`, never use `gorm.io/gorm/clause`/`Preload`.

The performance of ORMs is often quite inefficient. Therefore, adopt the data reading method that puts the least pressure on the database whenever possible. my philosophy is to use SQL for reading and reserve ORM for writing or modifying data.

Example:

```go
// When retrieving data, utilize Model/Find/First as much as possible,
// and rely on SQL for query conditions whenever you can.
db.Model(&User{}).
    Joins("JOIN emails ON emails.user_id = users.id AND emails.email = ?", "jinzhu@example.org").
    Joins("JOIN credit_cards ON credit_cards.user_id = users.id").Where("credit_cards.number = ?", "411111111111").
    Find(&user)

// Use Scan only when the data being read does not align with the database table structure.
db.Model(&User{}).
    Select("users.name AS name, emails.email AS email").
    Joins("left join emails on emails.user_id = users.id").
    Scan(&result{})

```
