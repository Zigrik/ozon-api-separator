# Ozon API Cabinet Separator

Система для разделения заказов Ozon FBS, управления маркировкой и этикетками.

## Возможности

- **Мультикабинетность** - работа с несколькими кабинетами Ozon
- **Автоматический режим** - полная автоматизация обработки заказов
- **Ручной режим** - управление через веб-интерфейс
- **Маркировка из .txt** - загрузка кодов маркировки из текстовых файлов
- **Этикетки** - заказ и скачивание PDF этикеток

## Требования

- Go 1.25.1+
- Доступ к API Ozon (Client-ID и API-Key)

## Установка

```bash
git clone https://github.com/your-repo/ozon-api-separator.git
cd ozon-api-separator
go mod download
Настройка
1. Файл .env
Создайте файл .env в корне проекта:

env
# Пароль для входа в веб-интерфейс
APP_PASSWORD=your_password

# Токен авторизации (генерируется автоматически)
AUTH_TOKEN=your_generated_token

# Пути (абсолютные или относительные)
LABELS_PATH=./labels
ORDERS_PATH=./orders
LOGS_PATH=./logs

# Настройка кабинетов
SHINORAMA_CLIENT_ID=your_client_id
SHINORAMA_API_KEY=your_api_key
SHINORAMA_AUTO_MODE=true

TRECKTRACK_CLIENT_ID=your_client_id
TRECKTRACK_API_KEY=your_api_key
TRECKTRACK_AUTO_MODE=false

SEVENHUNDREDSHIN_CLIENT_ID=your_client_id
SEVENHUNDREDSHIN_API_KEY=your_api_key
SEVENHUNDREDSHIN_AUTO_MODE=false

# Порт (по умолчанию 8080)
PORT=8080
2. Файл лицензии
Создайте файл license.key в корне проекта.

3. Коды маркировки (ручной режим)
Создайте GTINs.txt в корне проекта (один код на строку).

Структура проекта
text
ozon-api-separator/
├── internal/
│   ├── config/         # Конфигурация и загрузка .env
│   ├── handlers/       # HTTP обработчики
│   ├── middleware/     # Авторизация
│   ├── models/         # Структуры данных
│   └── services/       # Бизнес-логика
├── static/             # Статические файлы (CSS, images)
├── templates/          # HTML шаблоны
├── labels/             # Папка для этикеток и .txt файлов
│   ├── shinorama/
│   ├── trecktrack/
│   └── sevenhundredshin/
├── orders/             # JSON состояние заказов
├── logs/               # Логи
├── main.go
├── go.mod
├── .env
└── README.md
Формат .txt файлов
Файлы с расширением .txt помещаются в labels/кабинет/ или в подпапку заказа.

Формат строки (разделитель - два пробела):

text
номер_заказа  offer_id  код_маркировки
Пример:

text
0239848484-0062  106245  0102900059545041215hInVWrFI8ra_91EE1192liCxQaNVvhFzgaAOmmINeSxVDKLghvaDSn0b1R5GYaw=
Внимание: Марки могут содержать спецсимволы. Они сохраняются как есть.

Запуск
bash
go run main.go
Сервер будет доступен по адресу: http://localhost:8080

API Endpoints
Без авторизации
POST /api/check-password - проверка пароля

С авторизацией (X-Auth-Token)
POST /api/cabinet/switch - переключение кабинета

GET /api/orders - получение заказов

GET /api/orders/stats - статистика по кабинету

POST /api/orders/ship - разделение заказов

POST /api/orders/ship-and-labels - разделение + заказ этикеток

GET /api/orders/state - состояние заказа

GET /api/codes/available - количество доступных кодов

POST /api/codes/get - получение кодов маркировки

POST /api/codes/reload - перезагрузка кодов

POST /api/markings/add - добавление маркировки

POST /api/countries/set - установка страны

GET /api/countries/list - список стран

POST /api/gtd/absent - отметка ГТД

POST /api/labels/trigger-order - ручной заказ этикеток

POST /api/labels/trigger-download - ручное скачивание этикеток

POST /api/auto-mode/global - глобальный авто-режим

GET /api/auto-mode/global-status - статус авто-режима