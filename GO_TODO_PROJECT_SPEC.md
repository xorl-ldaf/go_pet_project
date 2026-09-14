# Go ToDo / Reminder Backend — Техническое задание и архитектура

> Рабочий архитектурный документ проекта.  
> Идейный референс: https://github.com/xorl-ldaf/ToDo-DevOps  
> Цель: сделать идейно похожий production-like backend на Go, но спроектировать его под особенности Go и использовать hexagonal architecture.

---

# 1. Цель проекта

Проект — многопользовательский backend для управления задачами и напоминаниями.

Пользователь может:

- зарегистрироваться и войти в систему;
- создавать задачи для себя;
- создавать задачи для другого пользователя, если имеет на это право;
- просматривать свои задачи;
- просматривать созданные им задачи;
- изменять разрешённые поля задачи;
- менять статус назначенной ему задачи;
- архивировать задачи без физического удаления;
- ставить один или несколько reminders;
- задавать reminders как на конкретное время, так и относительно deadline;
- создавать повторяющиеся задачи;
- получать внутренние уведомления;
- получать уведомления в Telegram;
- искать, фильтровать и сортировать задачи.

Проект должен быть достаточно серьёзным для портфолио/резюме и при этом оставаться учебным проектом, в котором осознанно отрабатываются:

1. Go;
2. backend architecture;
3. hexagonal architecture;
4. concurrency;
5. PostgreSQL;
6. Kafka;
7. background workers;
8. production-like инфраструктура;
9. тестирование;
10. CI.

Приоритеты проекта:

```text
Сильный проект для резюме
    >
Изучение Go
    >
Изучение backend architecture
    >
Изучение Kafka / event-driven
    >
Production-практики
```

---

# 2. Основная продуктовая идея

Главная сущность системы — `Task`.

У задачи есть:

- creator;
- assignee;
- title;
- description;
- status;
- дата создания;
- deadline;
- 0..N reminders;
- опциональная связь с recurring series.

Система разделяет несколько разных понятий:

```text
Task
    конкретная задача пользователя

TaskSeries
    правило генерации повторяющихся задач

Reminder
    правило: когда необходимо инициировать уведомление

Notification
    конкретное внутреннее уведомление пользователя

NotificationDelivery
    попытка доставки Notification во внешний канал
```

---

# 3. Пользовательские сценарии

## 3.1. Обычная задача

```text
1. Пользователь регистрируется.
2. Авторизуется.
3. Создаёт задачу "Сдать лабораторную".
4. Устанавливает deadline.
5. Добавляет reminders:
   - за 1 день;
   - за 2 часа.
6. В нужный момент backend создаёт уведомление.
7. Пользователь видит его внутри приложения.
8. Если подключён Telegram — получает сообщение туда.
9. После выполнения задачи assignee изменяет её статус.
```

## 3.2. Задача другому пользователю

```text
1. User A выбирает User B.
2. Backend проверяет, имеет ли A право назначать задачи B.
3. A создаёт Task:
       creator = A
       assignee = B
4. B видит задачу среди назначенных ему.
5. B может менять её status.
6. A сохраняет контроль над полями задачи, которыми владеет creator.
```

## 3.3. Повторяющаяся задача

```text
TaskSeries
    "Тренировка каждую неделю"

        ↓ генерирует

Task #1 — 01.09
Task #2 — 08.09
Task #3 — 15.09
...
```

Одна и та же `Task` не переиспользуется повторно.

Каждое повторение — отдельная Task.

---

# 4. Бизнес-правила

## 4.1. Creator и Assignee

У каждой задачи есть два пользователя:

```text
creator
    пользователь, создавший задачу

assignee
    пользователь, который должен её выполнить
```

Это могут быть:

```text
creator == assignee
```

или:

```text
creator != assignee
```

## 4.2. Права Creator

Creator может:

- менять `title`;
- менять `description`;
- менять `deadline`;
- менять `assignee`, если новое назначение разрешено permission system;
- изменять настройки reminders;
- изменять recurrence;
- архивировать задачу.

## 4.3. Права Assignee

Assignee может:

- видеть назначенную ему задачу;
- менять её `status`.

Assignee не должен автоматически получать право менять поля, принадлежащие creator.

## 4.4. Архивирование вместо удаления

Задачи физически не удаляются.

Не должно быть обычного hard delete бизнес-данных Task.

Используется явное бизнес-состояние архивирования:

```text
archived_at == NULL
    задача активна в обычном представлении

archived_at != NULL
    задача скрыта из обычных списков,
    но продолжает существовать и может быть запрошена
```

Обычный список задач не показывает архивные записи.

Архив можно просматривать отдельно.

## 4.5. Просроченность

`overdue` не является отдельным постоянным состоянием Task.

Она вычисляется:

```text
deadline < now
AND
task не находится в завершённом состоянии
```

То есть задача может одновременно иметь свой обычный status и быть `overdue`.

В БД отдельный `is_overdue` хранить не требуется.

## 4.6. Task Status

Статусы Task должны повторять модель исходного Java-проекта.

Перед реализацией:

```text
internal/task/domain/status.go
```

нужно перенести enum и допустимые переходы из Java-референса без придумывания альтернативной модели.

Это намеренно зафиксированное требование проекта.

## 4.7. Reminders

У одной задачи может быть несколько reminders.

Поддерживаются два типа.

### Absolute

```text
"Напомнить 18 сентября в 14:00"
```

### Relative

```text
"Напомнить за 2 часа до deadline"
```

При изменении deadline относительные reminders должны пересчитывать фактическое время срабатывания.

## 4.8. Завершённая задача

После завершения задачи оставшиеся несработавшие reminders должны перестать приводить к пользовательским уведомлениям.

Технически это можно реализовать отменой `PENDING` reminders.

## 4.9. Permission system

Первая версия использует прямые разрешения:

```text
User A → имеет право назначать Task User B
```

В будущем permission system должна расшириться до:

```text
Teams
Roles
Permissions / RBAC
```

Поэтому Task module не должен напрямую зависеть от таблицы `assignment_permissions`.

Он зависит только от абстракции:

```text
AssignmentAuthorizer
```

---

# 5. Внешний API — взгляд frontend

Основной транспорт V1:

```text
REST + JSON
```

HTTP реализуется на стандартном:

```text
net/http
```

Без Gin/Echo/Fiber.

Базовый namespace:

```text
/api/v1
```

---

# 6. API области

## 6.1. Auth

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
```

Пользовательская функциональность V1:

- registration;
- login.

Refresh/logout являются технической частью выбранной access + refresh token схемы.

## 6.2. Current User

```text
GET /api/v1/users/me
```

## 6.3. Assignable Users

```text
GET /api/v1/users/assignable
```

Endpoint возвращает только пользователей, которым текущий пользователь имеет право назначать задачи.

Frontend не должен самостоятельно воспроизводить permission rules.

## 6.4. Tasks

```text
POST  /api/v1/tasks
GET   /api/v1/tasks
GET   /api/v1/tasks/{id}
PATCH /api/v1/tasks/{id}
```

Отдельные бизнес-команды могут иметь собственные endpoints:

```text
POST /api/v1/tasks/{id}/complete
POST /api/v1/tasks/{id}/archive
POST /api/v1/tasks/{id}/restore
```

Hard-delete endpoint не требуется.

## 6.5. Task filtering

`GET /api/v1/tasks` должен в будущем поддерживать комбинации фильтров.

Минимальные направления:

- assigned to me;
- created by me;
- status;
- overdue;
- archived;
- deadline range;
- search;
- sort.

Примеры представлений frontend:

```text
Все мои задачи
Сегодня
На этой неделе
Просроченные
Завершённые
Назначенные мне
Созданные мной
Архив
```

## 6.6. Reminders

```text
GET    /api/v1/tasks/{taskId}/reminders
POST   /api/v1/tasks/{taskId}/reminders
PATCH  /api/v1/tasks/{taskId}/reminders/{reminderId}
DELETE /api/v1/tasks/{taskId}/reminders/{reminderId}
```

Удаление reminder допустимо.

Ограничение на отсутствие hard-delete относится к Task.

## 6.7. Notifications

```text
GET  /api/v1/notifications
GET  /api/v1/notifications/unread-count
POST /api/v1/notifications/{id}/read
POST /api/v1/notifications/read-all
```

V1 использует обычный REST polling.

Realtime через SSE/WebSocket оставляется на последующие версии.

## 6.8. Notification settings

Концептуальная область:

```text
GET/PATCH /api/v1/users/me/notification-settings
```

Каналы V1:

```text
INTERNAL
TELEGRAM
```

---

# 7. Authentication

Выбранная схема:

```text
Access JWT
+
Refresh Token
```

## Access token

- короткоживущий;
- используется в API requests;
- JWT.

## Refresh token

- хранится сервером;
- в PostgreSQL хранится hash токена, а не исходное значение;
- поддерживается expiration;
- поддерживается revoke;
- желательно использовать token rotation.

---

# 8. Хранилища

Основной source of truth:

```text
PostgreSQL
```

На V1 не требуется:

```text
Redis
отдельная DB для auth
отдельная DB для tasks
отдельная DB для notifications
```

Используется одна PostgreSQL database.

Kafka не является базой данных и не является source of truth.

---

# 9. Логическая структура PostgreSQL

```text
PostgreSQL
│
├── users
├── refresh_tokens
├── assignment_permissions
│
├── tasks
├── task_series
├── task_series_reminder_rules
├── reminders
│
├── notifications
├── notification_deliveries
├── telegram_links
│
├── outbox_events
└── processed_events
```

---

# 10. Таблица users

Основные поля:

```text
id
email
username
password_hash
timezone
created_at
updated_at
```

Требования:

- `id` — UUID;
- email unique;
- username unique;
- password хранится только как hash;
- timezone необходима для reminders и recurring tasks.

---

# 11. Таблица refresh_tokens

```text
id
user_id
token_hash
expires_at
revoked_at
created_at
```

Связь:

```text
users 1 ─── N refresh_tokens
```

---

# 12. Таблица tasks

```text
id
series_id

creator_id
assignee_id

title
description
status

deadline_at

created_at
updated_at
completed_at
archived_at
```

Связи:

```text
users 1 ─── N tasks
    как creator

users 1 ─── N tasks
    как assignee

task_series 1 ─── N tasks
```

`series_id` nullable для обычных задач.

---

# 13. Таблица assignment_permissions

```text
assigner_id
assignee_id
created_at
```

Семантика:

```text
assigner_id = A
assignee_id = B

=> A может назначать задачи B
```

Primary key:

```text
(assigner_id, assignee_id)
```

В будущем этот механизм может быть заменён или дополнен Teams/RBAC.

---

# 14. Таблица reminders

```text
id
task_id

kind

offset_seconds
trigger_at

state

created_at
sent_at
```

## kind

Минимально:

```text
ABSOLUTE
BEFORE_DEADLINE
```

## state

Минимально:

```text
PENDING
SENT
CANCELLED
```

Для relative reminder:

```text
trigger_at = task.deadline_at - offset
```

`trigger_at` хранится явно, чтобы scheduler мог быстро выбирать наступившие reminders.

---

# 15. Таблица task_series

`TaskSeries` — шаблон генерации повторяющихся задач.

Основные данные:

```text
id

creator_id
assignee_id

title
description

frequency
interval

next_deadline_at

timezone

ends_at
is_active

created_at
updated_at
```

Начальный набор frequency:

```text
DAILY
WEEKLY
MONTHLY
```

`interval` позволяет:

```text
1 DAY
2 WEEKS
3 MONTHS
```

Не использовать cron-expression как domain model V1.

---

# 16. Таблица task_series_reminder_rules

```text
id
series_id
offset_seconds
```

Используется для автоматического создания reminders каждой новой Task, созданной из `TaskSeries`.

---

# 17. Таблица notifications

```text
id
user_id

task_id
reminder_id

type

title
body

created_at
read_at
```

Reminder и Notification — разные сущности.

```text
Reminder
    определяет момент события

Notification
    пользовательское сообщение
```

Notification может в будущем создаваться не только из reminder.

Например:

```text
TASK_REMINDER
TASK_ASSIGNED
TASK_UPDATED
...
```

---

# 18. Таблица notification_deliveries

```text
id
notification_id

channel
status

attempts
next_attempt_at

sent_at
last_error
```

Канал:

```text
TELEGRAM
```

Internal notification существует непосредственно как запись `notifications`.

Внешняя доставка отслеживается отдельно.

---

# 19. Telegram

Таблица:

```text
telegram_links
```

Поля:

```text
user_id
chat_id
telegram_username
linked_at
enabled
```

Связь:

```text
User 1 ─── 0..1 TelegramLink
```

Bot token не хранится в пользовательских таблицах.

Он является application secret/config.

Telegram V1:

```text
только канал исходящих уведомлений
```

Будущая версия:

```text
Telegram может стать полноценным inbound interface
```

---

# 20. Transactional Outbox

Таблица:

```text
outbox_events
```

Поля:

```text
id
aggregate_type
aggregate_id
event_type
payload
created_at
published_at
attempts
```

Задача outbox:

не допустить ситуации:

```text
PostgreSQL commit успешен
Kafka publish потерян
```

Бизнес-изменение и запись outbox event выполняются в одной PostgreSQL transaction.

Отдельный Outbox Relay публикует события в Kafka.

---

# 21. Idempotent Kafka consumers

Таблица:

```text
processed_events
```

Поля:

```text
consumer_name
event_id
processed_at
```

Primary key:

```text
(consumer_name, event_id)
```

Позволяет корректно обрабатывать повторную доставку Kafka event.

---

# 22. Основные DB индексы

Предусмотреть индексы минимум для:

```text
tasks(assignee_id, status, deadline_at)

tasks(creator_id, created_at)

tasks(deadline_at)

reminders(trigger_at)
    WHERE state = PENDING

task_series(next_deadline_at)
    WHERE is_active = true

notifications(user_id, created_at)

notifications(user_id, created_at)
    WHERE read_at IS NULL

notification_deliveries(next_attempt_at)
    для недоставленных записей

outbox_events(created_at)
    WHERE published_at IS NULL
```

Позже для поиска задач можно добавить PostgreSQL Full Text Search / GIN.

---

# 23. Высокоуровневая runtime-архитектура

Проект содержит несколько executables:

```text
cmd/
├── api/
├── scheduler/
├── notifier/
└── dev/
```

## API process

Отвечает за:

- REST API;
- auth;
- users;
- permissions;
- tasks;
- reminders;
- notification reads/settings.

## Scheduler process

Отвечает за фоновые процессы:

```text
Reminder Scheduler
Recurrence Scheduler
Outbox Relay
```

Они работают параллельно внутри одного Go process.

## Notifier process

Отвечает за:

```text
Kafka consumer
создание/обработку notifications
Telegram delivery
delivery retry
idempotency
```

## Dev process

Отвечает только за удобство локальной разработки.

---

# 24. Runtime data flow

```text
                         Frontend
                            │
                         HTTP/JSON
                            │
                            ▼
                     ┌────────────┐
                     │    API     │
                     └─────┬──────┘
                           │
                           ▼
                      PostgreSQL
                           ▲
             ┌─────────────┴─────────────┐
             │                           │
             │                           │
      ┌──────┴──────┐             ┌──────┴──────┐
      │  Scheduler  │             │  Notifier   │
      │             │             │             │
      │ reminders   │             │ Kafka       │
      │ recurrence  │             │ consumer    │
      │ outbox      │             │ Telegram    │
      └──────┬──────┘             └──────▲──────┘
             │                           │
             └──────────► Kafka ─────────┘
```

---

# 25. Главный архитектурный стиль

Используется:

```text
Hexagonal Architecture
+
package by feature
```

Не используется глобальное техническое разделение вида:

```text
domain/
services/
repositories/
controllers/
```

Вместо этого каждая бизнес-область получает собственный маленький hexagon.

Пример:

```text
internal/task/
internal/reminder/
internal/notification/
```

---

# 26. Основные feature modules

```text
auth
user
permission
task
reminder
recurrence
notification
outbox
```

Технические области:

```text
platform
bootstrap
```

---

# 27. Полное дерево проекта

```text
todo-go/
│
├── cmd/
│   │
│   ├── api/
│   │   └── main.go
│   │
│   ├── scheduler/
│   │   └── main.go
│   │
│   ├── notifier/
│   │   └── main.go
│   │
│   └── dev/
│       └── main.go
│
├── internal/
│   │
│   ├── auth/
│   │   │
│   │   ├── domain/
│   │   │   ├── refresh_token.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   │
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── auth_service.go
│   │   │   │   │
│   │   │   │   └── out/
│   │   │   │       ├── user_repository.go
│   │   │   │       ├── refresh_token_repository.go
│   │   │   │       ├── password_hasher.go
│   │   │   │       └── token_provider.go
│   │   │   │
│   │   │   ├── command/
│   │   │   │   ├── register.go
│   │   │   │   ├── login.go
│   │   │   │   ├── refresh.go
│   │   │   │   └── logout.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── auth_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   └── http/
│   │       │       ├── routes.go
│   │       │       ├── handler.go
│   │       │       ├── middleware.go
│   │       │       ├── request.go
│   │       │       └── response.go
│   │       │
│   │       └── out/
│   │           ├── postgres/
│   │           │   ├── refresh_token_repository.go
│   │           │   ├── model.go
│   │           │   └── mapper.go
│   │           │
│   │           ├── jwt/
│   │           │   └── token_provider.go
│   │           │
│   │           └── password/
│   │               └── hasher.go
│   │
│   ├── user/
│   │   ├── domain/
│   │   │   ├── user.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── user_service.go
│   │   │   │   └── out/
│   │   │   │       └── user_repository.go
│   │   │   │
│   │   │   ├── query/
│   │   │   │   ├── get_me.go
│   │   │   │   └── search_users.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── user_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   └── http/
│   │       │       ├── routes.go
│   │       │       ├── handler.go
│   │       │       └── response.go
│   │       │
│   │       └── out/
│   │           └── postgres/
│   │               ├── repository.go
│   │               ├── model.go
│   │               └── mapper.go
│   │
│   ├── permission/
│   │   ├── domain/
│   │   │   ├── permission.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── permission_service.go
│   │   │   │   └── out/
│   │   │   │       └── permission_repository.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── permission_service.go
│   │   │
│   │   └── adapter/
│   │       └── out/
│   │           └── postgres/
│   │               ├── repository.go
│   │               └── model.go
│   │
│   ├── task/
│   │   │
│   │   ├── domain/
│   │   │   ├── task.go
│   │   │   ├── status.go
│   │   │   ├── rules.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   │
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── task_service.go
│   │   │   │   │
│   │   │   │   └── out/
│   │   │   │       ├── task_repository.go
│   │   │   │       └── assignment_authorizer.go
│   │   │   │
│   │   │   ├── command/
│   │   │   │   ├── create_task.go
│   │   │   │   ├── update_task.go
│   │   │   │   ├── change_status.go
│   │   │   │   ├── reassign_task.go
│   │   │   │   ├── archive_task.go
│   │   │   │   └── restore_task.go
│   │   │   │
│   │   │   ├── query/
│   │   │   │   ├── get_task.go
│   │   │   │   └── list_tasks.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── task_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   └── http/
│   │       │       ├── routes.go
│   │       │       ├── handler.go
│   │       │       ├── request.go
│   │       │       └── response.go
│   │       │
│   │       └── out/
│   │           └── postgres/
│   │               ├── repository.go
│   │               ├── model.go
│   │               ├── mapper.go
│   │               └── query.go
│   │
│   ├── reminder/
│   │   ├── domain/
│   │   │   ├── reminder.go
│   │   │   ├── kind.go
│   │   │   ├── state.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── reminder_service.go
│   │   │   │   └── out/
│   │   │   │       ├── reminder_repository.go
│   │   │   │       └── event_writer.go
│   │   │   │
│   │   │   ├── command/
│   │   │   │   ├── create_reminder.go
│   │   │   │   ├── update_reminder.go
│   │   │   │   └── cancel_reminder.go
│   │   │   │
│   │   │   ├── query/
│   │   │   │   └── list_reminders.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── reminder_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   ├── http/
│   │       │   │   ├── routes.go
│   │       │   │   ├── handler.go
│   │       │   │   ├── request.go
│   │       │   │   └── response.go
│   │       │   │
│   │       │   └── scheduler/
│   │       │       └── runner.go
│   │       │
│   │       └── out/
│   │           └── postgres/
│   │               ├── repository.go
│   │               ├── model.go
│   │               └── mapper.go
│   │
│   ├── recurrence/
│   │   ├── domain/
│   │   │   ├── series.go
│   │   │   ├── frequency.go
│   │   │   ├── reminder_rule.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   └── recurrence_service.go
│   │   │   │   └── out/
│   │   │   │       ├── series_repository.go
│   │   │   │       └── task_creator.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── recurrence_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   └── scheduler/
│   │       │       └── runner.go
│   │       └── out/
│   │           └── postgres/
│   │               ├── repository.go
│   │               ├── model.go
│   │               └── mapper.go
│   │
│   ├── notification/
│   │   │
│   │   ├── domain/
│   │   │   ├── notification.go
│   │   │   ├── delivery.go
│   │   │   ├── channel.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   ├── in/
│   │   │   │   │   ├── notification_service.go
│   │   │   │   │   └── event_consumer.go
│   │   │   │   │
│   │   │   │   └── out/
│   │   │   │       ├── notification_repository.go
│   │   │   │       ├── delivery_repository.go
│   │   │   │       └── message_sender.go
│   │   │   │
│   │   │   ├── command/
│   │   │   │   ├── create_notification.go
│   │   │   │   ├── mark_read.go
│   │   │   │   └── send_delivery.go
│   │   │   │
│   │   │   ├── query/
│   │   │   │   └── list_notifications.go
│   │   │   │
│   │   │   └── service/
│   │   │       └── notification_service.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   ├── http/
│   │       │   │   ├── routes.go
│   │       │   │   ├── handler.go
│   │       │   │   └── response.go
│   │       │   │
│   │       │   └── kafka/
│   │       │       ├── consumer.go
│   │       │       └── message.go
│   │       │
│   │       └── out/
│   │           ├── postgres/
│   │           │   ├── repository.go
│   │           │   ├── model.go
│   │           │   └── mapper.go
│   │           │
│   │           └── telegram/
│   │               └── sender.go
│   │
│   ├── outbox/
│   │   ├── application/
│   │   │   ├── port/
│   │   │   │   └── out/
│   │   │   │       ├── event_repository.go
│   │   │   │       └── event_publisher.go
│   │   │   └── service/
│   │   │       └── relay.go
│   │   │
│   │   └── adapter/
│   │       ├── in/
│   │       │   └── scheduler/
│   │       │       └── runner.go
│   │       └── out/
│   │           ├── postgres/
│   │           │   └── repository.go
│   │           └── kafka/
│   │               └── publisher.go
│   │
│   ├── platform/
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── database/
│   │   │   └── postgres.go
│   │   ├── kafka/
│   │   │   ├── producer.go
│   │   │   └── consumer.go
│   │   ├── logging/
│   │   │   └── logger.go
│   │   ├── metrics/
│   │   │   └── metrics.go
│   │   └── server/
│   │       └── http.go
│   │
│   └── bootstrap/
│       ├── api.go
│       ├── scheduler.go
│       └── notifier.go
│
├── migrations/
│   ├── 000001_users.up.sql
│   ├── 000001_users.down.sql
│   └── ...
│
├── deploy/
│   ├── compose.yaml
│   ├── prometheus/
│   └── grafana/
│
├── tests/
│   └── integration/
│
├── .github/
│   └── workflows/
│       └── ci.yml
│
├── .env.example
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

---

# 28. Hexagonal Architecture внутри каждого feature module

Общая форма:

```text
                   внешний мир

          inbound              outbound
          adapter               adapter
             │                    ▲
             ▼                    │
          port/in              port/out
             │                    ▲
             └──── application ───┘
                       │
                       ▼
                     domain
```

Важно:

`domain` находится в центре.

Технологии находятся снаружи.

---

# 29. Domain

Пример:

```text
internal/task/domain/
```

Domain содержит:

- entities;
- value-like domain types;
- бизнес-правила;
- допустимые переходы состояний;
- domain errors.

Domain не должен знать о:

```text
HTTP
JSON
GORM
PostgreSQL
Kafka
Telegram
JWT
Prometheus
Docker
```

Domain — обычные Go structs, types и methods.

Не создавать:

```text
Task interface
TaskImpl
```

если нет реальной причины.

Бизнес-сущность обычно является обычным concrete struct.

---

# 30. Application

Пример:

```text
internal/task/application/
```

Application содержит use cases.

Например:

```text
CreateTask
UpdateTask
ChangeStatus
ReassignTask
ArchiveTask
RestoreTask
GetTask
ListTasks
```

Application:

- координирует domain objects;
- вызывает domain rules;
- работает через ports;
- определяет транзакционные use cases;
- не знает конкретных infrastructure technologies.

---

# 31. Inbound Ports

Пример:

```text
internal/task/application/port/in/
```

Inbound port описывает:

```text
что внешний мир может попросить выполнить
```

В Go это обычно небольшой interface.

Например концептуально:

```text
TaskService:
    Create
    Update
    ChangeStatus
    Get
    List
```

Inbound adapter зависит от этого port.

---

# 32. Outbound Ports

Пример:

```text
internal/task/application/port/out/
```

Outbound port описывает:

```text
что application нужно от внешнего мира
```

Например:

```text
TaskRepository

AssignmentAuthorizer

EventWriter

TokenProvider

PasswordHasher

MessageSender
```

Очень важное Go-правило:

```text
interface объявляется рядом с потребителем,
а не рядом с реализацией
```

То есть `TaskRepository` принадлежит application layer, а не postgres package.

---

# 33. Inbound Adapters

Входы в hexagon.

Примеры:

```text
HTTP
Kafka consumer
Scheduler runner
CLI
Tests
```

### HTTP

```text
adapter/in/http/
├── routes.go
├── handler.go
├── request.go
└── response.go
```

Ответственности:

```text
routes.go
    URL → handler

handler.go
    parse transport request
    вызвать application
    преобразовать ошибку
    сформировать response

request.go
    HTTP/JSON DTO

response.go
    HTTP/JSON response DTO
```

HTTP adapter не содержит business rules.

### Kafka

```text
adapter/in/kafka/
```

Преобразует Kafka message в application command/use case.

### Scheduler

```text
adapter/in/scheduler/
```

Преобразует наступление времени/tick в вызов application use case.

---

# 34. Outbound Adapters

Примеры:

```text
PostgreSQL/GORM
Kafka producer
Telegram API
JWT
password hashing
```

### PostgreSQL adapter

```text
adapter/out/postgres/
├── repository.go
├── model.go
├── mapper.go
└── query.go
```

Ответственности:

```text
repository.go
    реализация application repository port

model.go
    GORM persistence model

mapper.go
    persistence model ↔ domain

query.go
    сложные SQL/GORM query builders
```

Domain objects не должны иметь GORM tags.

---

# 35. Как Go реализует ports без наследования

Go не требует:

```text
implements
extends
abstract class
```

Port — обычный interface.

Adapter — обычный struct.

Если struct имеет все методы interface, он автоматически удовлетворяет interface.

Поэтому связь выглядит так:

```text
task/application
    определяет TaskRepository

            ▲
            │ implicit interface satisfaction
            │

task/adapter/out/postgres
    Repository
```

Application не импортирует PostgreSQL adapter.

PostgreSQL adapter зависит от внутреннего контракта application.

Это и есть Dependency Inversion.

---

# 36. Composition вместо inheritance

Application service содержит зависимости:

```text
TaskService
│
├── TaskRepository
├── AssignmentAuthorizer
└── ...
```

Он не наследуется от:

```text
AbstractService
BaseService
RepositoryService
```

Основная модель Go:

```text
composition
+
small interfaces
+
explicit dependency injection
```

---

# 37. Dependency Injection

DI выполняется вручную.

Не используется Spring-подобный контейнер.

Все concrete implementations собираются в:

```text
internal/bootstrap/
```

`bootstrap` — composition root приложения.

Он знает одновременно:

```text
application ports
application services
HTTP adapters
Postgres adapters
Kafka adapters
Telegram adapters
platform
```

Остальные слои не должны знать весь dependency graph.

---

# 38. Bootstrap

```text
internal/bootstrap/
├── api.go
├── scheduler.go
└── notifier.go
```

## api.go

Собирает:

```text
config
logger
database

repositories

application services

HTTP handlers

routes

HTTP server
```

## scheduler.go

Собирает:

```text
Reminder Scheduler
Recurrence Scheduler
Outbox Relay
```

## notifier.go

Собирает:

```text
Kafka Consumer
Notification Application
Notification Repository
Delivery Repository
Telegram Sender
```

---

# 39. cmd

`cmd` содержит только точки запуска.

```text
cmd/api/main.go
cmd/scheduler/main.go
cmd/notifier/main.go
cmd/dev/main.go
```

`main.go` не должен содержать:

- business logic;
- repository queries;
- domain rules;
- HTTP handlers;
- Kafka processing logic.

Он только:

```text
загружает config
создаёт root context
инициализирует bootstrap
запускает process
обрабатывает shutdown
```

---

# 40. Связь файлов на примере Task request

Полный путь:

```text
Frontend
   │
   ▼
cmd/api/main.go
   │
   ▼
internal/bootstrap/api.go
   │
   ▼
internal/task/adapter/in/http/routes.go
   │
   ▼
internal/task/adapter/in/http/handler.go
   │
   ▼
internal/task/application/port/in/task_service.go
   │
   ▼
internal/task/application/service/task_service.go
   │
   ▼
internal/task/domain/*
   │
   ▼
internal/task/application/port/out/task_repository.go
   ▲
   │
internal/task/adapter/out/postgres/repository.go
   │
   ▼
GORM
   │
   ▼
PostgreSQL
```

Обратный путь:

```text
PostgreSQL
   ↓
postgres/model.go
   ↓
postgres/mapper.go
   ↓
domain.Task
   ↓
application
   ↓
http/response.go
   ↓
Frontend
```

---

# 41. Межмодульные зависимости

Modules не должны напрямую лезть во внутренности друг друга.

Например Task не должен знать:

```text
permission/adapter/out/postgres
assignment_permissions table
```

Task знает только:

```text
AssignmentAuthorizer
```

Permission module предоставляет подходящую реализацию.

Схема:

```text
task/application
      │
      ▼
AssignmentAuthorizer
      ▲
      │
permission/application
```

Связка выполняется в bootstrap.

---

# 42. Разрешённые зависимости

Разрешено:

```text
adapter/in → application port/in

adapter/in → transport DTO

application → domain

application → own ports

adapter/out → application port/out

adapter/out → domain
    только когда нужен mapping

bootstrap → application + adapters + platform
```

---

# 43. Запрещённые зависимости

Не должно быть:

```text
domain → application

domain → adapter

domain → GORM

domain → Kafka

domain → net/http

application → postgres adapter

application → HTTP adapter

application → Kafka adapter

application → Telegram adapter

task/domain → permission/postgres

notification/domain → Kafka
```

---

# 44. Platform

```text
internal/platform/
```

Это общий технический фундамент.

Разрешённые обязанности:

```text
config
database connection
Kafka clients
logging
metrics
HTTP server base
```

Platform не является местом для business logic.

Не создавать бесконтрольные:

```text
utils/
helpers/
common/
misc/
```

как свалку общего кода.

---

# 45. GORM

ORM:

```text
GORM
```

Но GORM ограничен persistence adapters.

Domain models не имеют GORM tags.

Persistence model и domain model разделены.

```text
domain.Task
        ↕ mapper
postgres.taskModel
```

---

# 46. Migrations

Использовать:

```text
golang-migrate
```

Миграции хранятся в:

```text
migrations/
```

Принцип:

```text
000001_*.up.sql
000001_*.down.sql
000002_*.up.sql
000002_*.down.sql
...
```

GORM AutoMigrate не является основным механизмом управления production schema.

---

# 47. Kafka

Kafka является учебной и архитектурной частью проекта.

Минимальный event flow V1:

```text
Reminder
    ↓
Outbox
    ↓
Kafka
    ↓
Notifier
    ↓
Notification / Telegram
```

Начальный topic:

```text
notification.requested.v1
```

Event должен иметь envelope минимум с:

```text
event_id
event_type
version
occurred_at
payload
```

Не публиковать всю domain model без необходимости.

---

# 48. Scheduler

Scheduler process содержит три независимых background component:

```text
Reminder Scheduler
Recurrence Scheduler
Outbox Relay
```

Они запускаются параллельно.

Go concurrency здесь используется по реальной необходимости, а не искусственно.

---

# 49. Reminder Scheduler

Основная задача:

```text
искать PENDING reminders,
для которых trigger_at наступил
```

Для безопасного параллелизма между несколькими scheduler instances использовать PostgreSQL locking по принципу:

```text
FOR UPDATE SKIP LOCKED
```

Scheduler должен поддерживать batch processing.

Для обработки batch можно применять ограниченный worker pool.

Не создавать неограниченное количество goroutines.

---

# 50. Recurrence Scheduler

Основная задача:

```text
найти TaskSeries,
для которых пора создать следующее occurrence
```

Должен:

1. безопасно захватить series;
2. создать новую Task;
3. создать reminders по reminder rules;
4. обновить next occurrence;
5. сделать операцию идемпотентной.

Повторная параллельная обработка не должна создавать duplicate Tasks.

---

# 51. Outbox Relay

Основная задача:

```text
читать unpublished outbox events
        ↓
publish Kafka
        ↓
помечать published
```

Outbox Relay работает независимо от API requests.

---

# 52. Notifier

Notifier является отдельным process.

Основной flow:

```text
Kafka
    ↓
notification/adapter/in/kafka
    ↓
notification/application
    ↓
PostgreSQL
    ↓
notification delivery
    ↓
Telegram
```

Kafka message подтверждается только после достаточной фиксации состояния обработки.

Повторный Kafka message не должен создавать повторное пользовательское действие.

---

# 53. Telegram Retry

Для Telegram требуется retry.

Концептуальный backoff:

```text
attempt 1
    ↓ fail
10 sec

attempt 2
    ↓ fail
1 min

attempt 3
    ↓ fail
5 min
```

Точные интервалы конфигурируются позже.

Retry не должен быть бесконечным.

Состояние попыток хранится в `notification_deliveries`.

---

# 54. Concurrency

Основные темы Go concurrency в проекте:

```text
context cancellation

errgroup

goroutines

channels

bounded worker pools

graceful shutdown

DB locking

Kafka consumer concurrency

race detector
```

Не использовать goroutines там, где обычный последовательный код проще и корректнее.

---

# 55. Graceful Shutdown

Каждый executable должен корректно реагировать на shutdown.

Закрываются:

```text
HTTP server

background loops

workers

Kafka clients

DB resources
```

Главный механизм координации:

```text
context.Context
```

---

# 56. Configuration

Конфигурация приходит через environment variables.

Локально допускается `.env`.

Основные группы:

```text
HTTP

Database

Kafka

JWT

Telegram

Scheduler

Logging

Metrics
```

Репозиторий содержит:

```text
.env.example
```

Secrets не коммитятся.

---

# 57. Logging

Использовать стандартный:

```text
log/slog
```

Требования:

- structured logs;
- request/event identifiers при необходимости;
- не использовать `fmt.Println` как основное приложение логирования;
- logger передаётся явной зависимостью там, где нужен.

---

# 58. Observability

Минимум:

```text
GET /healthz
GET /readyz
GET /metrics
```

Минимальные метрики:

```text
HTTP request count
HTTP request duration

reminders processed

Kafka events published

Kafka consumer processing

notifications created

Telegram delivery success

Telegram delivery failure
```

Инфраструктура:

```text
Prometheus
Grafana
```

Без обязательного OpenTelemetry/Jaeger/Loki на V1.

---

# 59. Локальная разработка

Предпочтительный development mode:

```text
Go processes:
    локально

Infrastructure:
    Docker
```

То есть:

```text
local:
    api
    scheduler
    notifier

Docker:
    PostgreSQL
    Kafka
    Prometheus
    Grafana
```

Это даёт быстрый feedback loop при разработке Go.

---

# 60. Production-like Docker mode

Должна также существовать возможность поднять весь stack через Docker:

```text
PostgreSQL
Kafka
API
Scheduler
Notifier
Prometheus
Grafana
```

Архитектура Go при переключении development → Docker не меняется.

---

# 61. cmd/dev

Пользователь проекта хочет возможность не запускать Docker инфраструктуру руками.

Поэтому:

```text
go run ./cmd/dev
```

должен концептуально:

1. проверить доступность Docker;
2. вызвать Docker Compose для infrastructure;
3. дождаться readiness PostgreSQL/Kafka;
4. запустить нужные Go processes;
5. передавать им shutdown;
6. корректно завершаться по Ctrl+C.

Не требуется писать собственный Docker orchestrator.

`cmd/dev` может использовать существующий Docker Compose CLI.

---

# 62. Docker Compose

`deploy/compose.yaml` должен позволять запускать:

```text
postgres
kafka
prometheus
grafana
api
scheduler
notifier
```

При разработке можно запускать только infrastructure subset.

---

# 63. Testing

Три основных уровня.

## Unit tests

Тестируют:

```text
domain rules
application use cases
```

External dependencies заменяются маленькими fake/mock implementations ports.

## Integration tests

Тестируют:

```text
PostgreSQL repositories
GORM mappings
locking
transactions
migrations
```

Использовать реальный PostgreSQL через containers/Testcontainers.

## HTTP tests

Тестируют:

```text
routes
handlers
request validation
response mapping
auth middleware
```

Использовать стандартные Go HTTP test tools.

---

# 64. Race testing

Так как проект сознательно использует concurrency, обязательна регулярная проверка:

```text
go test -race ./...
```

Race detector является частью качества проекта.

---

# 65. CI

GitHub Actions должен запускать минимум:

```text
gofmt check

go vet

golangci-lint

go test

go test -race

build api

build scheduler

build notifier
```

Позже:

```text
Docker image build
```

---

# 66. Основной стек

Зафиксированные технологии:

```text
Language:
    Go

HTTP:
    net/http

Architecture:
    Hexagonal Architecture
    package by feature

DB:
    PostgreSQL

ORM:
    GORM

Migrations:
    golang-migrate

Async:
    Kafka

Logging:
    log/slog

Metrics:
    Prometheus

Dashboards:
    Grafana

Containers:
    Docker
    Docker Compose

CI:
    GitHub Actions

Dependency Injection:
    manual

Config:
    environment variables
```

---

# 67. Почему нет `pkg/`

На старте проекту не требуется публичная reusable Go library.

Поэтому используется:

```text
internal/
```

`pkg/` не создаётся автоматически «потому что так делают Go-проекты».

Если когда-нибудь появится действительно reusable package для внешнего потребителя, его можно добавить отдельно.

---

# 68. Почему нет глобальных `repositories/` и `services/`

Не использовать структуру:

```text
internal/
├── controllers/
├── services/
├── repositories/
└── models/
```

Она группирует проект по техническим типам и делает feature boundaries размытыми.

Наша структура:

```text
internal/
├── task/
├── reminder/
├── notification/
└── ...
```

Каждый feature содержит собственный hexagon.

---

# 69. Правило Go interfaces

Главные правила:

1. Интерфейс создаётся там, где нужен потребителю.
2. Adapter не объявляет interface «для самого себя».
3. Interface должен быть маленьким.
4. Не делать interface для каждого struct.
5. Domain entities обычно concrete structs.
6. Не использовать Java-паттерн `XService + XServiceImpl` без причины.
7. Concrete adapter неявно удовлетворяет application port.
8. Manual DI связывает concrete implementation с port.

---

# 70. Основная dependency formula

Inbound:

```text
External World
      ↓
adapter/in
      ↓
port/in
      ↓
application
      ↓
domain
```

Outbound:

```text
application
      ↓
port/out
      ▲
      │
adapter/out
      ↓
External System
```

Compile-time dependency направлена внутрь.

---

# 71. Module: auth

Ответственность:

```text
registration
login
refresh token
logout/revoke
JWT creation/validation
password verification
```

Не отвечает за:

```text
Task
Reminder
Notification
```

Outbound dependencies:

```text
UserRepository
RefreshTokenRepository
PasswordHasher
TokenProvider
```

---

# 72. Module: user

Ответственность:

```text
User domain
current user information
user search
```

Не должен реализовывать Task permissions напрямую.

---

# 73. Module: permission

Ответственность V1:

```text
Can user A assign task to user B?
```

Будущее:

```text
Teams
Roles
RBAC
```

Это отдельная boundary, чтобы Task module не пришлось переписывать при появлении team model.

---

# 74. Module: task

Центральный бизнес-модуль.

Ответственность:

```text
Task lifecycle

creation

creator rules

assignee rules

status changes

reassignment

archiving/restoring

filtering/query contracts
```

Не знает:

```text
конкретную permission table
PostgreSQL
GORM
HTTP
Kafka
```

---

# 75. Module: reminder

Ответственность:

```text
absolute reminders

relative reminders

calculation of trigger time

reminder lifecycle

cancellation

due reminder use cases
```

Reminder не отвечает непосредственно за Telegram.

---

# 76. Module: recurrence

Ответственность:

```text
TaskSeries

frequency

next occurrence

generation of new Task

generation of reminder rules
```

TaskSeries не должна переиспользовать старую Task.

---

# 77. Module: notification

Ответственность:

```text
internal notifications

read/unread

notification delivery records

processing notification events

external message sending through a port
```

Не должен быть жёстко привязан только к Telegram.

Используется абстракция channel/message sender.

---

# 78. Module: outbox

Технический application module для надёжной доставки integration events.

Ответственность:

```text
unpublished events

event publication

mark as published

retry publication
```

---

# 79. Основные события

Минимально нужен event:

```text
notification.requested.v1
```

В будущем могут появиться:

```text
task.created
task.assigned
task.status.changed
task.archived
...
```

Но не добавлять события заранее без реальной необходимости.

---

# 80. V1 notification flow

```text
Reminder Scheduler
      ↓
Reminder due
      ↓
PostgreSQL transaction
      ↓
Outbox Event
      ↓
Outbox Relay
      ↓
Kafka
      ↓
Notifier
      ↓
Notification
      ↓
├── internal
└── Telegram delivery
```

---

# 81. V1 recurrence flow

```text
Recurrence Scheduler
      ↓
TaskSeries due
      ↓
transaction + lock
      ↓
create Task
      ↓
create Reminders
      ↓
advance next occurrence
```

---

# 82. V1 permission flow

```text
Task create/reassign
      ↓
Task Application
      ↓
AssignmentAuthorizer
      ↓
Permission Application
      ↓
Permission Repository
      ↓
PostgreSQL
```

Task application не читает permission table напрямую.

---

# 83. V1 auth flow

```text
Frontend
    ↓
Auth HTTP Adapter
    ↓
Auth Application
    ↓
├── UserRepository
├── PasswordHasher
├── TokenProvider
└── RefreshTokenRepository
```

---

# 84. План разработки

## Stage 0 — Foundation

Сделать:

```text
go module
directory structure

config
slog
PostgreSQL
GORM
migrations

health
ready
metrics skeleton

graceful shutdown

cmd/api
cmd/dev
```

Результат:

```text
приложение запускается
DB доступна
infrastructure поднимается
health работает
```

## Stage 1 — User + Auth

```text
User domain

registration
login

JWT access
refresh token

password hashing

auth middleware

GET /users/me
```

## Stage 2 — Task

```text
Task domain

status model from Java reference

business rules

GORM adapter

create

get

update

list

filters

sorting

archive/restore
```

## Stage 3 — Permissions

```text
assignment_permissions

AssignmentAuthorizer

assignable users

reassignment
```

## Stage 4 — Reminder

```text
Reminder domain

absolute

relative

CRUD

deadline recalculation
```

Пока без Kafka.

## Stage 5 — Scheduler

```text
cmd/scheduler

Reminder Scheduler

context

errgroup

worker pools

locking

SKIP LOCKED
```

## Stage 6 — Kafka + Outbox

```text
outbox_events

Outbox Relay

Kafka producer

notification.requested.v1
```

## Stage 7 — Notifications

```text
cmd/notifier

Kafka consumer

processed_events

internal notifications

read/unread API
```

## Stage 8 — Telegram

```text
Telegram linking

Telegram adapter

notification deliveries

retry
```

## Stage 9 — Recurrence

```text
TaskSeries

ReminderRules

Recurrence Scheduler

idempotent generation
```

## Stage 10 — Production polish

```text
Prometheus

Grafana

Docker

CI

integration tests

race tests

README

architecture diagrams
```

---

# 85. Что намеренно оставлено на более позднее проектирование

Следующие детали пока НЕ считаются окончательно выбранными:

## Exact TaskStatus enum

Нужно взять напрямую из Java-референса при реализации `task/domain/status.go`.

## Exact REST DTO schemas

Ресурсы и направления endpoints определены, но конкретные request/response JSON contracts будут проектироваться вместе с каждым module.

## Pagination

Нужна для Task/Notification list, но exact cursor/offset strategy ещё не выбрана.

## Teams/Roles

Планируются на будущее.

Текущий contract должен позволять добавить их без переписывания Task module.

## Telegram linking UX

Нужно отдельно выбрать handshake/linking flow.

## Exact retry policy

Конкретные числа retries/backoff будут определены позже.

## Kafka library

Kafka является выбранной технологией, но конкретный Go client library пока не зафиксирован.

## UUID library

Конкретная реализация UUID пока не принципиальна.

## Password hashing algorithm/library

Port определён концептуально; concrete implementation выбирается при реализации auth.

## Exact transaction abstraction

Нужно отдельно спроектировать так, чтобы application мог выражать транзакционные use cases без зависимости от GORM.

---

# 86. Архитектурные критерии готовности

Архитектура считается соблюдённой, если:

- domain не импортирует infrastructure;
- application не импортирует concrete adapters;
- ports принадлежат внутреннему слою;
- adapters реализуют ports;
- GORM models отделены от domain models;
- concrete dependency graph находится в bootstrap;
- HTTP handlers не содержат business logic;
- scheduler runners не содержат business logic;
- Kafka consumer не содержит domain rules;
- modules общаются через явные contracts;
- Task не знает устройство permission persistence;
- Reminder не знает устройство Telegram;
- PostgreSQL является source of truth;
- Kafka events обрабатываются идемпотентно;
- Task records не удаляются hard-delete;
- concurrency имеет bounded/controlled nature;
- shutdown контролируется context;
- integration tests работают с реальным PostgreSQL;
- race detector проходит.

---

# 87. Краткая архитектурная карта

```text
                             EXTERNAL WORLD

        Frontend              Scheduler                Kafka
           │                     │                       │
           ▼                     ▼                       ▼

      HTTP adapters       Scheduler adapters       Kafka adapters
           │                     │                       │
           └──────────────┬──────┴───────────────────────┘
                          ▼

                       PORTS / IN
                          │
                          ▼
                      APPLICATION
                          │
                          ▼
                        DOMAIN
                          ▲
                          │
                      PORTS / OUT
                          │
            ┌─────────────┼──────────────┐
            ▼             ▼              ▼

        PostgreSQL       Kafka        Telegram
         adapter         adapter        adapter
```

---

# 88. Ключевая идея проекта

Главная архитектурная мысль:

> HTTP, PostgreSQL, Kafka, Telegram и GORM — не приложение. Они являются адаптерами вокруг приложения.

В центре находятся:

```text
User
Task
Permission
Reminder
TaskSeries
Notification
```

И use cases, которые управляют ими.

Главная Go-мысль:

> Hexagonal Architecture в Go строится не через inheritance, а через направление зависимостей, маленькие interfaces, composition и manual dependency injection.

Главная structural формула:

```text
feature/
├── domain/
├── application/
│   └── port/
│       ├── in/
│       └── out/
└── adapter/
    ├── in/
    └── out/
```

Именно эта модель является архитектурной основой проекта.
