# Goodfeed Backend Migration Guide

> Migrating `goodfeed/backend` from `goamqp` to `gomessaging/amqp`

See [MIGRATION.md](./MIGRATION.md) for the complete API reference. This guide covers all goodfeed-specific files with concrete before/after examples.

## Overview

Goodfeed's backend is the heaviest consumer of goamqp:
- **14+ files** reference `goamqp`
- ~100 `WithTypeMapping` calls in `service.go`
- Multiple handler patterns: EventStream, ServiceRequest/Response, TypeMappingHandler, delayed publishing via sloth

### Key Patterns in Goodfeed

| Pattern | Files | Migration Effort |
|---------|-------|-----------------|
| `HandlerFunc` handlers | 10+ files | Medium — signature change |
| `Suffix()` methods | 6 files | Low — type rename |
| `Publisher.PublishWithContext` | 2 files | Medium — add routing key |
| `WithTypeMapping` calls | service.go | High volume but mechanical — delete all |
| `TypeMappingHandler` (readview `#` wildcard) | service.go | Medium — rewrite to package function |
| Sloth client integration | service.go, reminders/ | Medium — interface change |

---

## `cmd/service/service.go`

This is the main wiring file. It has the most changes.

### Import Changes

```go
// Before
import (
    "github.com/sparetimecoders/goamqp"
)

// After
import (
    "github.com/sparetimecoders/gomessaging/amqp"
    "github.com/sparetimecoders/gomessaging/spec"
)
```

### Connection Interface

```go
// Before
type Connection interface {
    Start(ctx context.Context, opts ...goamqp.Setup) error
    Close() error
    TypeMappingHandler(handler goamqp.HandlerFunc) goamqp.HandlerFunc
}

// After
type Connection interface {
    Start(ctx context.Context, opts ...amqp.Setup) error
    Close() error
    // TypeMappingHandler is removed — now a package-level function
}
```

### ConnectAMQP

```go
// Before
func ConnectAMQP(url string) (Connection, error) {
    return goamqp.NewFromURL(serviceName, url)
}

// After
func ConnectAMQP(url string) (*amqp.Connection, error) {
    return amqp.NewFromURL(serviceName, url)
}
```

> **Note:** You can use `*amqp.Connection` directly instead of the `Connection` interface, since the new API already satisfies all needed methods. If you keep the interface, remove `TypeMappingHandler` from it.

### Publisher Creation

```go
// Before
eventPublisher := goamqp.NewPublisher()
emailServicePublisher := goamqp.NewPublisher()
reportServicePublisher := goamqp.NewPublisher()

// After
eventPublisher := amqp.NewPublisher()
emailServicePublisher := amqp.NewPublisher()
reportServicePublisher := amqp.NewPublisher()
```

### Setup List — Remove All WithTypeMapping

All ~100 `goamqp.WithTypeMapping(...)` calls should be **deleted entirely**. The new API doesn't use type-based routing; consumers use generics and publishers use explicit routing keys.

```go
// DELETE ALL OF THESE:
goamqp.WithTypeMapping("Organization.Created", event.OrganizationCreated{}),
goamqp.WithTypeMapping("Report.Created", event.ReportCreated{}),
// ... (all ~100 lines)
```

### Setup List — Logger

```go
// Before
goamqp.UseLogger(func(msg string) {
    logger.Error(msg)
}),

// After
amqp.WithLogger(logger),
```

### Setup List — Prefetch & Close Listener

```go
// Before
goamqp.WithPrefetchLimit(20),
goamqp.CloseListener(closeEvents),

// After
amqp.WithPrefetchLimit(20),
amqp.CloseListener(closeEvents),
```

### Setup List — Event Stream Publisher

```go
// Before
goamqp.EventStreamPublisher(eventPublisher),

// After
amqp.EventStreamPublisher(eventPublisher),
```

### Setup List — Service Publishers

```go
// Before
goamqp.ServicePublisher("email-service", emailServicePublisher),
goamqp.ServicePublisher("report-service", reportServicePublisher),

// After
amqp.ServicePublisher("email-service", emailServicePublisher),
amqp.ServicePublisher("report-service", reportServicePublisher),
```

### Setup List — EventStreamConsumer (Generic, No Event Type)

Every `EventStreamConsumer` call loses the event type parameter and gains a type parameter:

```go
// Before
goamqp.EventStreamConsumer("Report.Created", reportingService.HandleReportCreated, event.ReportCreated{}, reportingService.Suffix()),
goamqp.EventStreamConsumer("MailStatus.Delivered", mailDeliveryService.Handle, delivery.DeliveredEvent{}),
goamqp.EventStreamConsumer("Survey.Sent", questionnaireHandler.Handle, event.SurveySent{}),

// After
amqp.EventStreamConsumer[event.ReportCreated]("Report.Created", reportingService.HandleReportCreated, reportingService.Suffix()),
amqp.EventStreamConsumer[delivery.DeliveredEvent]("MailStatus.Delivered", mailDeliveryService.Handle),
amqp.EventStreamConsumer[event.SurveySent]("Survey.Sent", questionnaireHandler.Handle),
```

**Full list of EventStreamConsumer rewrites:**

```go
// After — all EventStreamConsumer calls
amqp.EventStreamConsumer[event.ReportCreated]("Report.Created", reportingService.HandleReportCreated, reportingService.Suffix()),
amqp.EventStreamConsumer[delivery.DeliveredEvent]("MailStatus.Delivered", mailDeliveryService.Handle),
amqp.EventStreamConsumer[delivery.BouncedEvent]("MailStatus.Bounced", mailDeliveryService.Handle),
amqp.EventStreamConsumer[delivery.ComplaintEvent]("MailStatus.Complaint", mailDeliveryService.Handle),
amqp.EventStreamConsumer[handler.LogoUpdated]("Organization.LogoUpdated", fileUploadedHandler.Handle),
amqp.EventStreamConsumer[handler.UserProfileImageUpdated]("User.ProfileImageUpdated", fileUploadedHandler.Handle),
amqp.EventStreamConsumer[event.SurveySent]("Survey.Sent", questionnaireHandler.Handle),
amqp.EventStreamConsumer[event.QuestionnaireSent]("Questionnaire.Sent", questionnaireHandler.Handle),
amqp.EventStreamConsumer[event.SurveyResultUpdated]("Survey.ResultUpdated", questionnaireHandler.Handle),
amqp.EventStreamConsumer[event.FollowUpClosed]("FollowUp.Closed", questionnaireHandler.Handle),
amqp.EventStreamConsumer[event.QuestionnaireAnswered]("Questionnaire.Answered", questionnaireHandler.Handle),
amqp.EventStreamConsumer[event.QuestionnaireAnswered]("Questionnaire.Answered", retroHandler.Handle, retroHandler.Suffix()),
amqp.EventStreamConsumer[event.FollowUpClosed]("FollowUp.Closed", retroHandler.Handle, retroHandler.Suffix()),
amqp.EventStreamConsumer[event.ContactAddedToFollowUp]("FollowUp.ContactAdded", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.ScheduledSurveyAdded]("ScheduledSurvey.Added", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.ScheduledSurveyDateChanged]("ScheduledSurvey.SurveyDateChanged", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.FollowUpConverted]("FollowUp.Converted", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.FollowUpCreated]("FollowUp.Created", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.SurveyUpdated]("Survey.Updated", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.SurveyExpirationTimeExtended]("Survey.ExpirationTimeExtended", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.SurveySent]("Survey.Sent", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.QuestionnaireSent]("Questionnaire.Sent", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.SurveyReminderSent]("Survey.SurveyReminderSent", remindersHandler.HandleEvents, remindersHandler.Suffix()),
amqp.EventStreamConsumer[event.UserRemoved]("Organization.UserRemoved", userHandler.Handle, userHandler.Suffix()),
amqp.EventStreamConsumer[event.UserRolesAdded]("Organization.UserRolesAdded", notificationHandler.HandleEvents, notificationHandler.Suffix()),
amqp.EventStreamConsumer[event.UserRoleRemoved]("Organization.UserRoleRemoved", notificationHandler.HandleEvents, notificationHandler.Suffix()),
```

### Setup List — TransientEventStreamConsumer

```go
// Before
goamqp.TransientEventStreamConsumer("Notification.Created", notificationHandler.HandleEvents, event.NotificationCreated{}),
goamqp.TransientEventStreamConsumer("Notification.Dismissed", notificationHandler.HandleEvents, event.NotificationDismissed{}),
goamqp.TransientEventStreamConsumer("Organization.UserAdded", rolesHandler.HandleEvents, event.UserAdded{}),
goamqp.TransientEventStreamConsumer("Organization.UserRemoved", rolesHandler.HandleEvents, event.UserRemoved{}),
goamqp.TransientEventStreamConsumer("Organization.UserRolesAdded", rolesHandler.HandleEvents, event.UserRolesAdded{}),
goamqp.TransientEventStreamConsumer("Organization.UserRoleRemoved", rolesHandler.HandleEvents, event.UserRoleRemoved{}),

// After
amqp.TransientEventStreamConsumer[event.NotificationCreated]("Notification.Created", notificationHandler.HandleEvents),
amqp.TransientEventStreamConsumer[event.NotificationDismissed]("Notification.Dismissed", notificationHandler.HandleEvents),
amqp.TransientEventStreamConsumer[event.UserAdded]("Organization.UserAdded", rolesHandler.HandleEvents),
amqp.TransientEventStreamConsumer[event.UserRemoved]("Organization.UserRemoved", rolesHandler.HandleEvents),
amqp.TransientEventStreamConsumer[event.UserRolesAdded]("Organization.UserRolesAdded", rolesHandler.HandleEvents),
amqp.TransientEventStreamConsumer[event.UserRoleRemoved]("Organization.UserRoleRemoved", rolesHandler.HandleEvents),
```

### Setup List — ServiceResponseConsumer

```go
// Before
goamqp.ServiceResponseConsumer("email-service", "email.send", mailService.HandleEmailSent, mail.SendEmailResponse{}),
goamqp.ServiceResponseConsumer("report-service", "report.generate", reportingService.HandleReportResponse, reporting.ReportResponse{}),

// After
amqp.ServiceResponseConsumer[mail.SendEmailResponse]("email-service", "email.send", mailService.HandleEmailSent),
amqp.ServiceResponseConsumer[reporting.ReportResponse]("report-service", "report.generate", reportingService.HandleReportResponse),
```

### Setup List — ServiceRequestConsumer (Readview.Reset)

The inline handler must be updated to the new signature:

```go
// Before
goamqp.ServiceRequestConsumer("Readview.Reset", func(msg any, headers goamqp.Headers) (response any, err error) {
    if ev, ok := msg.(*readview.Reset); ok {
        err := rv.Reset(context.Background(), ev.Name)
        return nil, err
    }
    logger.Warn(fmt.Sprintf("got unexpected message: '%s'", reflect.TypeOf(msg).String()))
    return nil, nil
}, readview.Reset{}),

// After
amqp.ServiceRequestConsumer[readview.Reset]("Readview.Reset", func(ctx context.Context, event spec.ConsumableEvent[readview.Reset]) error {
    return rv.Reset(ctx, event.Payload.Name)
}),
```

> **Note:** With generics, the payload is already typed as `readview.Reset` — no type assertion needed.

### Setup List — Readview `#` Wildcard Consumer (TypeMappingHandler)

This is the most complex rewrite. The old code used `conn.TypeMappingHandler(handler)` as a Connection method. The new API uses `amqp.TypeMappingHandler(handler, mapper)` as a package-level function.

```go
// Before
for k, v := range rv.Handlers() {
    name := k
    messageHandler := v
    setups = append(setups,
        goamqp.EventStreamConsumer("#", rv.RegularMappingHandler(conn, name, messageHandler), json.RawMessage{}, goamqp.AddQueueNameSuffix(fmt.Sprintf("readview-%s", k))),
    )
}

// After — build a TypeMapper from event types registered in the event store
typeMapper := func(routingKey string) (reflect.Type, bool) {
    // Build this map from the same event types registered in pg.New()
    // See the readview migration guide for details on building the TypeMapper
    t, ok := eventTypeMap[routingKey]
    return t, ok
}

for k, v := range rv.Handlers() {
    name := k
    messageHandler := v
    handler := rv.RegularMappingHandler(name, messageHandler)
    wrappedHandler := amqp.TypeMappingHandler(handler, typeMapper)
    setups = append(setups,
        amqp.EventStreamConsumer[any]("#", wrappedHandler, amqp.AddQueueNameSuffix(fmt.Sprintf("readview-%s", k))),
    )
}
```

> **Important:** The `RegularMappingHandler` and `ExternalMappingHandler` functions in the readview package must also be updated — they no longer take a `Connection` parameter. See [readview.md](./readview.md) for details.

### Setup List — Sloth ResponseListener

```go
// Before
delayPublisher.ResponseListener(remindersHandler.HandleReminders,
    reminders.RemindProjectManagerAboutSurvey{},
    reminders.RemindFollowupManagerAboutSurvey{},
    reminders.BatchEmail{},
    reminders.RemindContactAboutQuestionnaire{},
    reminders.ScheduleSendSurvey{},
    reminders.ScheduleCompleteSurvey{},
),

// After — handler signature must change first (see reminders section below)
delayPublisher.ResponseListener(remindersHandler.HandleReminders,
    sloth.Mapping{Type: reminders.RemindProjectManagerAboutSurvey{}, Key: "Reminders.RemindProjectManagerAboutSurvey"},
    sloth.Mapping{Type: reminders.RemindFollowupManagerAboutSurvey{}, Key: "Reminders.RemindFollowupManagerAboutSurvey"},
    sloth.Mapping{Type: reminders.BatchEmail{}, Key: "Reminders.BatchEmail"},
    sloth.Mapping{Type: reminders.RemindContactAboutQuestionnaire{}, Key: "Reminders.RemindContactAboutQuestionnaire"},
    sloth.Mapping{Type: reminders.ScheduleSendSurvey{}, Key: "Reminders.ScheduleSendSurvey"},
    sloth.Mapping{Type: reminders.ScheduleCompleteSurvey{}, Key: "Reminders.ScheduleCompleteSurvey"},
),
```

> **Note:** The `ResponseListener` signature in `sloth/client-amqp` also changes. The handler parameter becomes `spec.EventHandler[any]`. See [sloth.md](./sloth.md) for details. The call site at goodfeed currently passes types directly (not `Mapping` structs) to `ResponseListener` — this will need to match the updated sloth client API.

---

## Handler Files — Signature Changes

All handler functions must change from the old `HandlerFunc` signature to the new generic `EventHandler[T]` signature.

### Pattern: Simple Handler (most files)

```go
// Before — goamqp.HandlerFunc
func (h *Handler) Handle(msg interface{}, _ goamqp.Headers) (interface{}, error) {
    switch m := msg.(type) {
    case *event.SomeEvent:
        // ...
    }
    return nil, nil
}

// After — spec.EventHandler[T] (called from generic consumer)
// Option A: Keep as any-typed handler (when consumer uses EventStreamConsumer[SomeEvent])
// The handler receives already-typed payload, no switch needed for single-type consumers.
// For handlers that serve multiple routing keys with different types, use any:
func (h *Handler) Handle(ctx context.Context, event spec.ConsumableEvent[any]) error {
    switch m := event.Payload.(type) {
    case *event.SomeEvent:
        // ...
    }
    return nil
}
```

However, since each `EventStreamConsumer[T]` call specifies a concrete type, the handler will receive the payload already deserialized as that type. **If a handler function is only called with one type**, you can make it type-specific. Since goodfeed reuses the same handler function across multiple `EventStreamConsumer` calls with different types (e.g., `questionnaireHandler.Handle` handles `SurveySent`, `QuestionnaireSent`, etc.), the handler must remain `any`-typed.

### `reminders/handler.go`

This file has the most complex handler changes due to the `SlothClient` interface and `Suffix()` return type.

#### SlothClient Interface

```go
// Before
type SlothClient interface {
    Close() error
    ResponseListener(handler goamqp.HandlerFunc, types ...interface{}) goamqp.Setup
    Publish(request interface{}, when time.Time) error
}

// After
type SlothClient interface {
    Close() error
    ResponseListener(handler spec.EventHandler[any], mappings ...sloth.Mapping) amqp.Setup
    Publish(request interface{}, when time.Time) error
}
```

#### Handler Interface

```go
// Before
type Handler interface {
    HandleEvents(msg interface{}, _ goamqp.Headers) (_ interface{}, err error)
    HandleReminders(msg interface{}, headers goamqp.Headers) (response interface{}, err error)
    Suffix() goamqp.QueueBindingConfigSetup
}

// After
type Handler interface {
    HandleEvents(ctx context.Context, event spec.ConsumableEvent[any]) error
    HandleReminders(ctx context.Context, event spec.ConsumableEvent[any]) error
    Suffix() amqp.ConsumerOptions
}
```

#### Suffix()

```go
// Before
func (h *handler) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("reminders")
}

// After
func (h *handler) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("reminders")
}
```

#### HandleEvents

```go
// Before
func (h *handler) HandleEvents(msg interface{}, _ goamqp.Headers) (_ interface{}, err error) {
    ctx := context.Background()
    a := msg.(eventsourced.Event)
    aggregateId := a.AggregateIdentity()
    switch m := a.(type) {
    case *event.FollowUpConverted:
        // ...
    case *event.SurveySent:
        if err := h.slothClient.Publish(&ScheduleCompleteSurvey{...}, ...); err != nil {
            return nil, err
        }
    }
    return nil, nil
}

// After
func (h *handler) HandleEvents(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    a := evt.Payload.(eventsourced.Event)
    aggregateId := a.AggregateIdentity()
    switch m := a.(type) {
    case *event.FollowUpConverted:
        // ...
    case *event.SurveySent:
        if err := h.slothClient.Publish(&ScheduleCompleteSurvey{...}, ...); err != nil {
            return err
        }
    }
    return nil
}
```

> **Key changes:** `msg` → `evt.Payload`, `ctx := context.Background()` → use `ctx` from parameter, return `error` only (no response).

#### HandleReminders

```go
// Before
func (h *handler) HandleReminders(msg interface{}, _ goamqp.Headers) (response interface{}, err error) {
    ctx := context.Background()
    switch m := msg.(type) {
    case *RemindContactAboutQuestionnaire:
        // ...
        return nil, nil
    case *BatchEmail:
        // ...
        return nil, err
    }
    return nil, nil
}

// After
func (h *handler) HandleReminders(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch m := evt.Payload.(type) {
    case *RemindContactAboutQuestionnaire:
        // ...
        return nil
    case *BatchEmail:
        // ...
        return err
    }
    return nil
}
```

---

### `handler/user_handler.go`

```go
// Before
func (u *UserHandler) Handle(msg interface{}, _ goamqp.Headers) (interface{}, error) {
    ctx := context.Background()
    switch m := msg.(type) {
    case *event.UserRemoved:
        // ...
    }
    return nil, nil
}

func (u *UserHandler) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("users")
}

// After
func (u *UserHandler) Handle(ctx context.Context, evt spec.ConsumableEvent[event.UserRemoved]) error {
    m := &evt.Payload
    userId := m.UserId
    organizationId := m.AggregateIdentity()
    // ... rest of logic unchanged
    return nil
}

func (u *UserHandler) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("users")
}
```

> **Note:** Since `UserHandler.Handle` is only called with `event.UserRemoved`, it can use a concrete type parameter instead of `any`.

---

### `mail/delivery/handler.go`

This handler handles three types (`DeliveredEvent`, `BouncedEvent`, `ComplaintEvent`), so it must stay `any`-typed:

```go
// Before
func (h *handler) Handle(msg any, headers goamqp.Headers) (any, error) {
    ctx := context.Background()
    var reason *string
    var messageId string
    switch evt := msg.(type) {
    case *DeliveredEvent:
        messageId = evt.MessageId
    // ...
    }
    // ...
    return nil, nil
}

// After
func (h *handler) Handle(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    var reason *string
    var messageId string
    switch e := evt.Payload.(type) {
    case *DeliveredEvent:
        messageId = e.MessageId
    case *BouncedEvent:
        messageId = e.MessageId
        reason = &e.Reason
    case *ComplaintEvent:
        messageId = e.MessageId
        reason = &e.Reason
    }
    // ... rest unchanged, return nil instead of (nil, nil)
    return nil
}
```

---

### `reporting/reporting.go`

Two handler changes plus a publisher change:

```go
// Before
func (h *reporting) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("reporting")
}

func (r *reporting) HandleReportCreated(msg any, _ goamqp.Headers) (any, error) {
    data := msg.(*event.ReportCreated)
    err := r.publisher.PublishWithContext(context.Background(), &ReportRequest{data})
    return nil, err
}

func (h *reporting) HandleReportResponse(msg any, _ goamqp.Headers) (any, error) {
    resp := msg.(*ReportResponse)
    return h.updater.UpdateReport(context.Background(), resp.Id, resp.UsedPrompt, resp.Result)
}

// After
func (h *reporting) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("reporting")
}

func (r *reporting) HandleReportCreated(ctx context.Context, evt spec.ConsumableEvent[event.ReportCreated]) error {
    return r.publisher.Publish(ctx, "report.generate", &ReportRequest{&evt.Payload})
}

func (h *reporting) HandleReportResponse(ctx context.Context, evt spec.ConsumableEvent[reporting.ReportResponse]) error {
    _, err := h.updater.UpdateReport(ctx, evt.Payload.Id, evt.Payload.UsedPrompt, evt.Payload.Result)
    return err
}
```

Also update the struct to use the new Publisher type:

```go
// Before
type reporting struct {
    publisher *goamqp.Publisher
    // ...
}

// After
type reporting struct {
    publisher *amqp.Publisher
    // ...
}
```

> **Key change:** `publisher.PublishWithContext(ctx, &ReportRequest{data})` → `publisher.Publish(ctx, "report.generate", &ReportRequest{data})` — the routing key is now explicit.

---

### `mail/mail.go`

```go
// Before
type Publisher interface {
    PublishWithContext(context.Context, any, ...goamqp.Header) error
}

func (m *mailer) HandleEmailSent(msg any, headers goamqp.Headers) (any, error) {
    ctx := context.Background()
    // ...
    return nil, nil
}

// After
type Publisher interface {
    Publish(context.Context, string, any, ...amqp.Header) error
}

func (m *mailer) HandleEmailSent(ctx context.Context, evt spec.ConsumableEvent[SendEmailResponse]) error {
    response := &evt.Payload
    logger := m.logger.With("trackingId", response.TrackingId).With("messageId", response.MessageId)
    logger.Debug("email sent")
    _, err := m.db.ExecContext(ctx, "UPDATE email_delivery SET message_id=$1 WHERE tracking_id=$2", response.MessageId, response.TrackingId)
    if err != nil {
        logger.With("err", err).ErrorContext(ctx, "failed to associate trackingId")
        return err
    }
    return nil
}
```

Also update `SendEmail` to use explicit routing key:

```go
// Before
err = m.publisher.PublishWithContext(ctx, &SendEmailRequest{...})

// After
err = m.publisher.Publish(ctx, "email.send", &SendEmailRequest{...})
```

---

### `subscriptions/notifications/notifications.go`

```go
// Before
func (h *Handler) HandleEvents(msg any, _ goamqp.Headers) (response any, err error) {
    ctx := context.Background()
    switch e := msg.(type) {
    case *event.UserRolesAdded:
        // ...
    case *event.NotificationCreated:
        // ...
    }
    return nil, nil
}

func (h *Handler) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("notification-handler")
}

// After
func (h *Handler) HandleEvents(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch e := evt.Payload.(type) {
    case *event.UserRolesAdded:
        // ... (use ctx from parameter)
    case *event.NotificationCreated:
        // ...
    }
    return nil
}

func (h *Handler) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("notification-handler")
}
```

---

### `subscriptions/roles/roles.go`

```go
// Before
func (h *Handler) HandleEvents(msg any, _ goamqp.Headers) (response any, err error) {
    ctx := context.Background()
    switch e := msg.(type) {
    case *event.UserAdded:
        // ...
        return h.HandleEvents(msg, nil)  // recursive retry
    }
    return nil, nil
}

func (h *Handler) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("roles-handler")
}

// After
func (h *Handler) HandleEvents(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch e := evt.Payload.(type) {
    case *event.UserAdded:
        // ...
        return h.HandleEvents(ctx, evt)  // recursive retry
    }
    return nil
}

func (h *Handler) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("roles-handler")
}
```

---

### `handler/questionnaire_handler.go`

```go
// Before
func (s *QuestionnaireHandler) Handle(msg interface{}, _ goamqp.Headers) (interface{}, error) {
    ctx := context.Background()
    switch m := msg.(type) {
    case *event.SurveySent:
        // ...
    }
    return nil, nil
}

// After
func (s *QuestionnaireHandler) Handle(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch m := evt.Payload.(type) {
    case *event.SurveySent:
        surveyId := m.AggregateIdentity()
        return s.service.SurveySendQuestionnaires(ctx, surveyId.String())
    case *event.QuestionnaireSent:
        return s.service.QuestionnaireSend(ctx, m.Questionnaire)
    case *event.QuestionnaireAnswered:
        return s.service.QuestionnaireAnswered(ctx, m.Questionnaire)
    case *event.SurveyResultUpdated:
        // ...
    case *event.FollowUpClosed:
        // ...
    default:
        s.logger.ErrorContext(ctx, fmt.Sprintf("unexpected message (%s): %v", reflect.TypeOf(evt.Payload).String(), evt.Payload))
    }
    return nil
}
```

---

### `handler/retro_handler.go`

```go
// Before
func (s *RetroHandler) Handle(msg interface{}, _ goamqp.Headers) (interface{}, error) {
    ctx := context.Background()
    switch m := msg.(type) {
    case *event.QuestionnaireAnswered:
        return s.service.RetroQuestionnaireAnswered(ctx, m.Questionnaire)
    case *event.FollowUpClosed:
        // ...
    }
    return nil, nil
}

func (h *RetroHandler) Suffix() goamqp.QueueBindingConfigSetup {
    return goamqp.AddQueueNameSuffix("retros")
}

// After
func (s *RetroHandler) Handle(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch m := evt.Payload.(type) {
    case *event.QuestionnaireAnswered:
        _, err := s.service.RetroQuestionnaireAnswered(ctx, m.Questionnaire)
        return err
    case *event.FollowUpClosed:
        id := m.AggregateIdentity()
        followUp, err := s.service.FollowUpById(ctx, id.String())
        if err != nil {
            return err
        }
        return s.service.RetroClose(ctx, followUp.SurveyIds...)
    }
    return nil
}

func (h *RetroHandler) Suffix() amqp.ConsumerOptions {
    return amqp.AddQueueNameSuffix("retros")
}
```

---

### `handler/fileupload_handler.go`

```go
// Before
func (s *FileUploadedHandler) Handle(msg interface{}, _ goamqp.Headers) (interface{}, error) {
    ctx := context.Background()
    switch m := msg.(type) {
    case *LogoUpdated:
        err := s.service.OrganizationUpdateLogoUrl(ctx, m.ID, m.LogotypeURL)
        if err != nil {
            return nil, err
        }
    case *UserProfileImageUpdated:
        // ...
    default:
        s.logger.Error(fmt.Sprintf("Got unexpected message (%s): %v", reflect.TypeOf(msg).String(), msg))
    }
    return nil, nil
}

// After
func (s *FileUploadedHandler) Handle(ctx context.Context, evt spec.ConsumableEvent[any]) error {
    switch m := evt.Payload.(type) {
    case *LogoUpdated:
        return s.service.OrganizationUpdateLogoUrl(ctx, m.ID, m.LogotypeURL)
    case *UserProfileImageUpdated:
        return s.service.UserUpdatePictureUrl(ctx, m.ID, m.ImageURL)
    default:
        s.logger.Error(fmt.Sprintf("Got unexpected message (%s): %v", reflect.TypeOf(evt.Payload).String(), evt.Payload))
    }
    return nil
}
```

---

## ErrRecoverable Replacement

The old API had `goamqp.ErrRecoverable` — wrapping an error with it suppressed logging but still nack+requeued. The new API does **not** have this sentinel error. All errors cause nack+requeue.

The readview package uses `ErrRecoverable` in its mapping handlers. After migration, the error will still be nacked and requeued — the only difference is that it will also be logged. Use slog log levels to control noise.

```go
// Before
return response, fmt.Errorf("%w: %v", goamqp.ErrRecoverable, err)

// After — just return the error, it will be nacked and requeued
return fmt.Errorf("recoverable: %w", err)
```

---

## Migration Checklist

### Phase 1: Update Dependencies
- [ ] Add `github.com/sparetimecoders/gomessaging/amqp` and `github.com/sparetimecoders/gomessaging/spec` to `go.mod`
- [ ] Remove `github.com/sparetimecoders/goamqp` from `go.mod`

### Phase 2: Update `cmd/service/service.go`
- [ ] Update imports
- [ ] Update `Connection` interface (remove `TypeMappingHandler`)
- [ ] Update `ConnectAMQP` function
- [ ] Replace `goamqp.NewPublisher()` → `amqp.NewPublisher()`
- [ ] Replace `UseLogger` → `WithLogger`
- [ ] Replace all `EventStreamConsumer` calls (add type param, remove event type arg)
- [ ] Replace all `TransientEventStreamConsumer` calls
- [ ] Replace all `ServiceResponseConsumer` calls
- [ ] Rewrite `ServiceRequestConsumer` for `Readview.Reset`
- [ ] Rewrite readview `#` wildcard consumers with `TypeMappingHandler`
- [ ] Delete all `WithTypeMapping` calls (~100 lines)
- [ ] Update `ServicePublisher` and `EventStreamPublisher` calls
- [ ] Update sloth `ResponseListener` call

### Phase 3: Update Handler Files
- [ ] `reminders/handler.go` — Update `Handler` interface, `SlothClient` interface, `HandleEvents`, `HandleReminders`, `Suffix()`
- [ ] `handler/user_handler.go` — Update `Handle`, `Suffix()`
- [ ] `mail/delivery/handler.go` — Update `Handle`
- [ ] `reporting/reporting.go` — Update `HandleReportCreated`, `HandleReportResponse`, `Suffix()`, publisher calls
- [ ] `mail/mail.go` — Update `Publisher` interface, `HandleEmailSent`, `SendEmail`
- [ ] `subscriptions/notifications/notifications.go` — Update `HandleEvents`, `Suffix()`
- [ ] `subscriptions/roles/roles.go` — Update `HandleEvents`, `Suffix()`
- [ ] `handler/questionnaire_handler.go` — Update `Handle`
- [ ] `handler/retro_handler.go` — Update `Handle`, `Suffix()`
- [ ] `handler/fileupload_handler.go` — Update `Handle`

### Phase 4: Update Readview Integration
- [ ] Update readview's `RegularMappingHandler` / `ExternalMappingHandler` (see [readview.md](./readview.md))
- [ ] Build the `TypeMapper` from event types

### Phase 5: Infrastructure
- [ ] Delete old classic queues before deploying (new module uses quorum queues)
- [ ] Consider `amqp.WithLegacySupport()` if rolling deployment with old publishers
- [ ] Test end-to-end with all handlers

### Phase 6: Optional New Features
- [ ] Add `amqp.WithTracing(tp)` for OTel trace propagation
- [ ] Add `amqp.WithLegacySupport()` for backward-compatible CloudEvents enrichment
- [ ] Add `amqp.WithNotificationChannel(ch)` for observability
