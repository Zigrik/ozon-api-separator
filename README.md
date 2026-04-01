# OZON Order Cabinet

Управление заказами Ozon FBS: разделение заказов, маркировка Честный ЗНАК, печать этикеток.

## Возможности

- Загрузка заказов в статусе `awaiting_packaging`
- Разделение заказов по 1 товару в отправление
- Добавление кодов маркировки (из файла GTINs.txt) и отметка об отсутствии ГТД
- Установка страны производителя
- Автоматическое скачивание этикеток через 60 секунд после разделения
- Поддержка нескольких кабинетов Ozon

## Настройка .env

```env
# Порт сервера
PORT=8080

# Пароль для входа
APP_PASSWORD=your_password

# Токен автоматической авторизации (любая строка)
AUTH_TOKEN=your_token

# Текст на оверлее при отправке
LOADING_TEXT=Трудолюбивые ослики делят и сортируют ваши заказы...

# Путь для сохранения этикеток (если не указан — папка labels)
LABELS_PATH=C:/Users/User/Documents/OzonLabels

# Кабинеты (названия по ключам)
SHINORAMA_CLIENT_ID=123456
SHINORAMA_API_KEY=abc-123

TRECKTRACK_CLIENT_ID=789012
TRECKTRACK_API_KEY=def-456

SEVENHUNDREDSHIN_CLIENT_ID=345678
SEVENHUNDREDSHIN_API_KEY=ghi-789