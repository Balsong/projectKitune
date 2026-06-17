#!/usr/bin/env bash
# ============================================================
# КИЦУНЭ — e2e-тесты всех функций сайта через web-слой (nginx :8081):
# раздача страниц/ассетов, /api-прокси и сквозные потоки (меню, корзина,
# заказ + сага, бронь, статусы). Тестирует ровно тот путь, что видит браузер.
#
# Запуск:  make up   (или docker compose up -d)  — поднять стек
#          tests/e2e.sh            — прогон против http://localhost:8081
#          BASE=http://host tests/e2e.sh   — другой адрес
# Код выхода 0 — все тесты прошли, иначе 1.
# ============================================================
set -uo pipefail

BASE="${BASE:-http://localhost:8081}"
PASS=0; FAIL=0; FAILED_NAMES=()

c_green=$'\e[32m'; c_red=$'\e[31m'; c_dim=$'\e[2m'; c_bold=$'\e[1m'; c_off=$'\e[0m'

ok()   { PASS=$((PASS+1)); printf "  ${c_green}✓${c_off} %s\n" "$1"; }
bad()  { FAIL=$((FAIL+1)); FAILED_NAMES+=("$1"); printf "  ${c_red}✗ %s${c_off}\n" "$1"; [ -n "${2:-}" ] && printf "      ${c_dim}%s${c_off}\n" "$2"; }
sect() { printf "\n${c_bold}%s${c_off}\n" "$1"; }

# HTTP status code of a GET
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
# JSON field via python: jget '<json>' '<expr over d>'
# Выражение передаётся через argv (не интерполируется в исходник),
# поэтому кавычки внутри выражения (d['items']) не ломают парсинг.
jget() { python3 -c '
import sys, json
try:
    d = json.loads(sys.stdin.read())
    print(eval(sys.argv[1]))
except Exception:
    print("__ERR__")
' "$2" <<<"$1"; }

# assert that GET path returns 200
page() { local p="$1"; local hc; hc=$(code "$BASE$p"); [ "$hc" = "200" ] && ok "GET $p → 200" || bad "GET $p → $hc (ожидали 200)"; }

CART="e2e-$$-$RANDOM"   # уникальная корзина на прогон
H=(-H "Content-Type: application/json" -H "X-Cart-Id: $CART")

# ---------- 0. доступность ----------
sect "0 · Доступность стека"
if [ "$(code "$BASE/api/v1/menu")" != "200" ]; then
  printf "  ${c_red}Стек не отвечает на %s/api/v1/menu.${c_off}\n  Подними: ${c_bold}make up${c_off} (или docker compose up -d), затем повтори.\n" "$BASE"
  exit 1
fi
ok "web+gateway доступны ($BASE)"

# ---------- 1. страницы ----------
sect "1 · Раздача страниц (статика)"
for p in / /index.html /menu-tea.html /menu-kitchen.html /promos.html /about.html \
         /contacts.html /reservation.html /delivery.html /cart.html /account.html; do
  page "$p"
done
# несуществующий путь — nginx отдаёт index.html (SPA-fallback)
hc=$(code "$BASE/no-such-page-xyz"); [ "$hc" = "200" ] && ok "неизвестный путь → 200 (fallback)" || bad "fallback → $hc"

# ---------- 2. ассеты ----------
sect "2 · Статические ассеты"
for a in /css/kitsune.css /js/kitsune.js /js/catalog.js /assets/logo.svg; do
  page "$a"
done

# ---------- 3. меню (каталог) ----------
sect "3 · Меню / каталог (/api/v1/menu)"
MENU=$(curl -s "$BASE/api/v1/menu")
N=$(jget "$MENU" "len(d)")
[ "$N" -ge 20 ] 2>/dev/null && ok "меню отдаёт $N позиций" || bad "меню: позиций $N (ожидали ≥20)"
FIRST_SKU=$(jget "$MENU" "[x['sku'] for x in d if x.get('sku')][0]")
HAS_FIELDS=$(jget "$MENU" "all(k in d[0] for k in ('id','sku','name','price_cents','kind'))")
[ "$HAS_FIELDS" = "True" ] && ok "позиция содержит id/sku/name/price_cents/kind" || bad "в позиции нет нужных полей"
# чай и блюда присутствуют
KINDS=$(jget "$MENU" "len({x['kind'] for x in d})")
[ "$KINDS" -ge 2 ] 2>/dev/null && ok "есть и чай, и блюда ($KINDS вида)" || bad "видов товара: $KINDS"

# id хитового блюда для дальнейших тестов
DISH_ID=$(jget "$MENU" "[x['id'] for x in d if x['sku']=='cof-cappuccino'][0]")
TEA_ID=$(jget "$MENU" "[x['id'] for x in d if x['sku']=='cha-red'][0]")

# ---------- 4. корзина ----------
sect "4 · Корзина (/api/v1/cart)"
EMPTY=$(curl -s "${H[@]}" "$BASE/api/v1/cart")
[ "$(jget "$EMPTY" "len(d.get('items',[]))")" = "0" ] && ok "новая корзина пуста" || bad "новая корзина не пуста"

R=$(curl -s -X POST "${H[@]}" -d "{\"product_id\":\"$DISH_ID\",\"quantity\":2}" "$BASE/api/v1/cart/items")
[ "$(jget "$R" "d['items'][0]['quantity']")" = "2" ] && ok "add: блюдо ×2" || bad "add блюда не сработал" "$R"

R=$(curl -s -X POST "${H[@]}" -d "{\"product_id\":\"$TEA_ID\",\"quantity\":1}" "$BASE/api/v1/cart/items")
[ "$(jget "$R" "len(d['items'])")" = "2" ] && ok "add: вторая позиция (чай)" || bad "вторая позиция не добавилась" "$R"

TOTAL=$(jget "$R" "d['total_cents']")
[ "$TOTAL" -gt 0 ] 2>/dev/null && ok "итог корзины посчитан ($TOTAL коп)" || bad "итог корзины = $TOTAL"

R=$(curl -s -X DELETE "${H[@]}" -d "{\"product_id\":\"$DISH_ID\",\"quantity\":1}" "$BASE/api/v1/cart/items")
DQ=$(jget "$R" "[i['quantity'] for i in d['items'] if i['product_id']=='$DISH_ID'][0]")
[ "$DQ" = "1" ] && ok "remove: количество блюда уменьшилось до 1" || bad "remove не уменьшил количество" "$R"

R=$(curl -s -X POST "${H[@]}" "$BASE/api/v1/cart/clear")
[ "$(jget "$R" "len(d.get('items',[]))")" = "0" ] && ok "clear: корзина очищена" || bad "clear не очистил корзину"

# валидация: несуществующий товар → 400
hc=$(code -X POST "${H[@]}" -d '{"product_id":"00000000-0000-0000-0000-000000000000","quantity":1}' "$BASE/api/v1/cart/items")
[ "$hc" = "400" ] && ok "add несуществующего товара → 400" || bad "несуществующий товар → $hc (ожидали 400)"

# ---------- 5. оформление заказа + сага ----------
sect "5 · Оформление заказа и сага"
# пустая корзина → 409
hc=$(code -X POST "${H[@]}" -d '{"fulfillment_type":"food_courier","address":""}' "$BASE/api/v1/orders")
[ "$hc" = "409" ] && ok "checkout пустой корзины → 409" || bad "пустой checkout → $hc (ожидали 409)"

# неверный способ получения → 400
curl -s -X POST "${H[@]}" -d "{\"product_id\":\"$TEA_ID\",\"quantity\":1}" "$BASE/api/v1/cart/items" >/dev/null
hc=$(code -X POST "${H[@]}" -d '{"fulfillment_type":"teleport","address":""}' "$BASE/api/v1/orders")
[ "$hc" = "400" ] && ok "неверный fulfillment_type → 400" || bad "неверный fulfillment → $hc (ожидали 400)"

# успешный checkout
ORD=$(curl -s -X POST "${H[@]}" -d '{"fulfillment_type":"food_courier","address":"ул. Тестовая, 1"}' "$BASE/api/v1/orders")
OID=$(jget "$ORD" "d['id']")
[ -n "$OID" ] && [ "$OID" != "__ERR__" ] && ok "заказ создан ($OID)" || { bad "заказ не создан" "$ORD"; OID=""; }

if [ -n "$OID" ]; then
  [ "$(jget "$ORD" "d['status']")" = "ORDER_STATUS_CREATED" ] && ok "начальный статус CREATED" || bad "начальный статус не CREATED"
  # корзина очищена после оформления
  [ "$(jget "$(curl -s "${H[@]}" "$BASE/api/v1/cart")" "len(d.get('items',[]))")" = "0" ] && ok "корзина очищена после заказа" || bad "корзина не очищена после заказа"

  # ждём прохождения саги до completed
  FINAL="";
  for i in $(seq 1 30); do
    S=$(jget "$(curl -s "$BASE/api/v1/orders/$OID")" "d['status']")
    if [ "$S" = "ORDER_STATUS_COMPLETED" ]; then FINAL="$S"; break; fi
    sleep 2
  done
  [ "$FINAL" = "ORDER_STATUS_COMPLETED" ] && ok "сага дошла до COMPLETED (≤60с)" || bad "сага не завершилась, последний статус: ${S:-?}"

  # доставка создана
  DLV=$(curl -s "$BASE/api/v1/orders/$OID/delivery")
  DS=$(jget "$DLV" "d['status']")
  [ "$DS" = "DELIVERY_STATUS_DELIVERED" ] && ok "доставка DELIVERED" || bad "статус доставки: $DS"
fi

# ---------- 6. компенсация (превышение лимита оплаты) ----------
sect "6 · Компенсация саги (отказ оплаты)"
CART="e2e-fail-$$-$RANDOM"; H=(-H "Content-Type: application/json" -H "X-Cart-Id: $CART")
# дорогой авторский чай ×11 (95 000 × 11 = 1 045 000 коп) превышает лимит mock-оплаты (1 000 000)
EXP_ID=$(jget "$MENU" "[x['id'] for x in d if x['sku']=='auth-dhp'][0]")
curl -s -X POST "${H[@]}" -d "{\"product_id\":\"$EXP_ID\",\"quantity\":11}" "$BASE/api/v1/cart/items" >/dev/null
OID2=$(jget "$(curl -s -X POST "${H[@]}" -d '{"fulfillment_type":"food_courier","address":"x"}' "$BASE/api/v1/orders")" "d['id']")
if [ -n "$OID2" ] && [ "$OID2" != "__ERR__" ]; then
  FINAL2=""
  for i in $(seq 1 20); do
    S=$(jget "$(curl -s "$BASE/api/v1/orders/$OID2")" "d['status']")
    if [ "$S" = "ORDER_STATUS_PAYMENT_FAILED" ] || [ "$S" = "ORDER_STATUS_CANCELLED" ]; then FINAL2="$S"; break; fi
    [ "$S" = "ORDER_STATUS_COMPLETED" ] && { FINAL2="$S"; break; }
    sleep 2
  done
  [ "$FINAL2" = "ORDER_STATUS_PAYMENT_FAILED" ] && ok "крупный заказ отклонён (PAYMENT_FAILED, компенсация)" || bad "ожидали PAYMENT_FAILED, получили: ${FINAL2:-$S}"
else
  bad "не удалось создать заказ для теста компенсации"
fi

# ---------- 7. бронирование ----------
sect "7 · Бронирование столика (/api/v1/book)"
BK=$(curl -s -X POST -H "Content-Type: application/json" -d '{"customer":"E2E Тест","phone":"+7 999 000-00-00","guests":2,"time":"2026-07-01 19:00","comment":"у окна"}' "$BASE/api/v1/book")
BID=$(jget "$BK" "d.get('booking_id','')")
[ -n "$BID" ] && [ "$BID" != "__ERR__" ] && ok "бронь принята (booking_id=$BID)" || bad "бронь не принята" "$BK"
[ "$(jget "$BK" "d.get('status')")" = "booking_pending" ] && ok "статус брони booking_pending" || bad "неожиданный статус брони"
# гостевая бронь без имени → 400
hc=$(code -X POST -H "Content-Type: application/json" -d '{"customer":"","time":""}' "$BASE/api/v1/book")
[ "$hc" = "400" ] && ok "бронь без имени/времени → 400" || bad "пустая бронь → $hc (ожидали 400)"
# /bookings без токена → 401
hc=$(code "$BASE/api/v1/bookings")
[ "$hc" = "401" ] && ok "/bookings без токена → 401" || bad "/bookings без токена → $hc (ожидали 401)"

# ---------- 8. аккаунт: регистрация / вход / сессия ----------
sect "8 · Аккаунт (/api/v1/auth)"
AEMAIL="e2e-$$-$RANDOM@kitsune.tea"
REG=$(curl -s -X POST -H "Content-Type: application/json" \
  -d "{\"email\":\"$AEMAIL\",\"password\":\"secret123\",\"name\":\"E2E\",\"phone\":\"+70000000000\",\"consent_personal_data\":true}" \
  "$BASE/api/v1/auth/register")
ATOKEN=$(jget "$REG" "d.get('session_token','')")
[ -n "$ATOKEN" ] && [ "$ATOKEN" != "__ERR__" ] && ok "регистрация (с согласием ПДн) → токен" || bad "регистрация не вернула токен" "$REG"
[ "$(jget "$REG" "d['user']['email']")" = "$(printf '%s' "$AEMAIL" | tr 'A-Z' 'a-z')" ] && ok "в ответе корректный пользователь" || bad "пользователь в ответе неверный"

# регистрация без согласия → 400
hc=$(code -X POST -H "Content-Type: application/json" -d "{\"email\":\"x$AEMAIL\",\"password\":\"secret123\",\"consent_personal_data\":false}" "$BASE/api/v1/auth/register")
[ "$hc" = "400" ] && ok "регистрация без согласия ПДн → 400" || bad "без согласия → $hc (ожидали 400)"

# дубликат e-mail → 409
hc=$(code -X POST -H "Content-Type: application/json" -d "{\"email\":\"$AEMAIL\",\"password\":\"secret123\",\"consent_personal_data\":true}" "$BASE/api/v1/auth/register")
[ "$hc" = "409" ] && ok "повторный e-mail → 409" || bad "дубль e-mail → $hc (ожидали 409)"

# me с токеном → пользователь
[ "$(jget "$(curl -s "$BASE/api/v1/auth/me" -H "Authorization: Bearer $ATOKEN")" "d.get('email','')")" = "$(printf '%s' "$AEMAIL" | tr 'A-Z' 'a-z')" ] && ok "me с токеном → текущий пользователь" || bad "me не вернул пользователя"

# заказ под пользователем виден в истории (/api/v1/orders)
ACART="e2e-acc-$$-$RANDOM"
curl -s -X POST -H "Content-Type: application/json" -H "X-Cart-Id: $ACART" -d "{\"product_id\":\"$TEA_ID\",\"quantity\":1}" "$BASE/api/v1/cart/items" >/dev/null
curl -s -X POST -H "Content-Type: application/json" -H "X-Cart-Id: $ACART" -H "Authorization: Bearer $ATOKEN" -d '{"fulfillment_type":"food_courier","address":"ul"}' "$BASE/api/v1/orders" >/dev/null
MINE=$(jget "$(curl -s "$BASE/api/v1/orders" -H "Authorization: Bearer $ATOKEN")" "len(d)")
[ "$MINE" -ge 1 ] 2>/dev/null && ok "заказ привязан к пользователю (история /orders: $MINE)" || bad "история заказов пуста"
hc=$(code "$BASE/api/v1/orders")
[ "$hc" = "401" ] && ok "/orders без токена → 401" || bad "/orders без токена → $hc (ожидали 401)"

# бронь под пользователем видна в истории (/api/v1/bookings)
curl -s -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $ATOKEN" \
  -d '{"customer":"E2E Аккаунт","phone":"+70000000000","guests":3,"time":"2026-07-02 20:00","comment":""}' "$BASE/api/v1/book" >/dev/null
MYBK=$(jget "$(curl -s "$BASE/api/v1/bookings" -H "Authorization: Bearer $ATOKEN")" "len(d)")
[ "$MYBK" -ge 1 ] 2>/dev/null && ok "бронь привязана к пользователю (история /bookings: $MYBK)" || bad "история броней пуста"

# вход верный → токен
LToken=$(jget "$(curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$AEMAIL\",\"password\":\"secret123\"}" "$BASE/api/v1/auth/login")" "d.get('session_token','')")
[ -n "$LToken" ] && [ "$LToken" != "__ERR__" ] && ok "вход с верным паролем → токен" || bad "вход не вернул токен"

# вход неверный → 401
hc=$(code -X POST -H "Content-Type: application/json" -d "{\"email\":\"$AEMAIL\",\"password\":\"wrong\"}" "$BASE/api/v1/auth/login")
[ "$hc" = "401" ] && ok "вход с неверным паролем → 401" || bad "неверный пароль → $hc (ожидали 401)"

# logout, затем me → 401
curl -s -X POST "$BASE/api/v1/auth/logout" -H "Authorization: Bearer $ATOKEN" >/dev/null
hc=$(code "$BASE/api/v1/auth/me" -H "Authorization: Bearer $ATOKEN")
[ "$hc" = "401" ] && ok "после logout сессия недействительна (me → 401)" || bad "me после logout → $hc (ожидали 401)"

# страницы входа/регистрации отдаются
for p in /login.html /register.html; do page "$p"; done

# ---------- 9. админка ----------
sect "9 · Админ-панель (/api/v1/admin)"
# админ из ADMIN_EMAILS (compose: admin@kitsune.tea). Регистрируем (или входим).
ADMIN_EMAIL="admin@kitsune.tea"; ADMIN_PASS="admin123"
curl -s -X POST -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASS\",\"name\":\"Admin\",\"consent_personal_data\":true}" \
  "$BASE/api/v1/auth/register" >/dev/null
ADM=$(curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASS\"}" "$BASE/api/v1/auth/login")
ADMTOKEN=$(jget "$ADM" "d.get('session_token','')")
[ "$(jget "$ADM" "d['user'].get('role','')")" = "admin" ] && ok "пользователь из ADMIN_EMAILS получает роль admin" || bad "роль admin не назначена" "$ADM"

# обычный (не админ) пользователь для проверки 403
UEMAIL="e2e-user-$$-$RANDOM@kitsune.tea"
UREG=$(curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$UEMAIL\",\"password\":\"secret123\",\"consent_personal_data\":true}" "$BASE/api/v1/auth/register")
UTOKEN=$(jget "$UREG" "d.get('session_token','')")

# доступ к ручкам админки
[ "$(code "$BASE/api/v1/admin/orders")" = "401" ] && ok "/admin/orders без токена → 401" || bad "/admin/orders без токена не 401"
UC=$(code "$BASE/api/v1/admin/orders" -H "Authorization: Bearer $UTOKEN")
[ "$UC" = "403" ] && ok "/admin/orders под обычным пользователем → 403" || bad "/admin/orders под user не 403" "got=$UC"
AO=$(curl -s "$BASE/api/v1/admin/orders" -H "Authorization: Bearer $ADMTOKEN")
[ "$(jget "$AO" "isinstance(d,list)")" = "True" ] && ok "/admin/orders под админом → список заказов" || bad "/admin/orders под админом не список" "$AO"
AB=$(curl -s "$BASE/api/v1/admin/bookings" -H "Authorization: Bearer $ADMTOKEN")
[ "$(jget "$AB" "isinstance(d,list)")" = "True" ] && ok "/admin/bookings под админом → список броней" || bad "/admin/bookings под админом не список" "$AB"

# смена статуса брони (используем BID из раздела 7)
if [ -n "${BID:-}" ] && [ "$BID" != "__ERR__" ]; then
  UB=$(curl -s -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $ADMTOKEN" -d '{"status":"confirmed"}' "$BASE/api/v1/admin/bookings/$BID/status")
  [ "$(jget "$UB" "d.get('status')")" = "confirmed" ] && ok "админ подтверждает бронь (confirmed)" || bad "смена статуса брони не сработала" "$UB"
fi

# редактирование позиции меню (цена не меняется, доступность включена)
TPRICE=$(jget "$MENU" "[x['price_cents'] for x in d if x['id']=='$TEA_ID'][0]")
UP=$(curl -s -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $ADMTOKEN" -d "{\"price_cents\":$TPRICE,\"available\":true}" "$BASE/api/v1/admin/menu/$TEA_ID")
[ "$(jget "$UP" "d.get('available')")" = "True" ] && ok "админ редактирует позицию меню (UpdateProduct)" || bad "обновление позиции не сработало" "$UP"
# изменение видно в публичном меню
[ "$(jget "$(curl -s "$BASE/api/v1/menu")" "[x['available'] for x in d if x['id']=='$TEA_ID'][0]")" = "True" ] && ok "изменение меню видно публично (кэш сброшен)" || bad "изменение меню не отразилось в /menu"

# страница админки отдаётся
page "/admin.html"

# ---------- 10. безопасность аккаунтов ----------
sect "10 · Безопасность (смена пароля, защита от брутфорса)"

# смена пароля: регистрируем пользователя, меняем пароль
PEMAIL="e2e-pwd-$$-$RANDOM@kitsune.tea"
PREG=$(curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$PEMAIL\",\"password\":\"oldpass123\",\"consent_personal_data\":true}" "$BASE/api/v1/auth/register")
PTOKEN=$(jget "$PREG" "d.get('session_token','')")
# неверный текущий пароль → 403
hc=$(code -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $PTOKEN" -d '{"old_password":"wrongpass","new_password":"newpass123"}' "$BASE/api/v1/auth/change-password")
[ "$hc" = "403" ] && ok "смена пароля с неверным текущим → 403" || bad "неверный текущий пароль → $hc (ожидали 403)"
# слишком короткий новый → 400
hc=$(code -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $PTOKEN" -d '{"old_password":"oldpass123","new_password":"123"}' "$BASE/api/v1/auth/change-password")
[ "$hc" = "400" ] && ok "короткий новый пароль → 400" || bad "короткий пароль → $hc (ожидали 400)"
# без токена → 401
hc=$(code -X POST -H "Content-Type: application/json" -d '{"old_password":"oldpass123","new_password":"newpass123"}' "$BASE/api/v1/auth/change-password")
[ "$hc" = "401" ] && ok "смена пароля без токена → 401" || bad "без токена → $hc (ожидали 401)"
# корректная смена → 200, вход новым работает, старым — нет
hc=$(code -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $PTOKEN" -d '{"old_password":"oldpass123","new_password":"newpass123"}' "$BASE/api/v1/auth/change-password")
[ "$hc" = "200" ] && ok "смена пароля с верным текущим → 200" || bad "смена пароля → $hc (ожидали 200)"
NLOGIN=$(curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$PEMAIL\",\"password\":\"newpass123\"}" "$BASE/api/v1/auth/login")
NT=$(jget "$NLOGIN" "d.get('session_token','')")
[ -n "$NT" ] && [ "$NT" != "__ERR__" ] && ok "вход новым паролем работает" || bad "вход новым паролем не сработал"
hc=$(code -X POST -H "Content-Type: application/json" -d "{\"email\":\"$PEMAIL\",\"password\":\"oldpass123\"}" "$BASE/api/v1/auth/login")
[ "$hc" = "401" ] && ok "вход старым паролем → 401" || bad "старый пароль → $hc (ожидали 401)"

# защита от брутфорса: после 5 неудач вход блокируется (429)
LEMAIL="e2e-lock-$$-$RANDOM@kitsune.tea"
curl -s -X POST -H "Content-Type: application/json" -d "{\"email\":\"$LEMAIL\",\"password\":\"secret123\",\"consent_personal_data\":true}" "$BASE/api/v1/auth/register" >/dev/null
for i in 1 2 3 4 5; do
  code -X POST -H "Content-Type: application/json" -d "{\"email\":\"$LEMAIL\",\"password\":\"wrong$i\"}" "$BASE/api/v1/auth/login" >/dev/null
done
# 6-я попытка даже с верным паролем → 429 (аккаунт временно заблокирован)
hc=$(code -X POST -H "Content-Type: application/json" -d "{\"email\":\"$LEMAIL\",\"password\":\"secret123\"}" "$BASE/api/v1/auth/login")
[ "$hc" = "429" ] && ok "после 5 неудач вход заблокирован (429)" || bad "брутфорс-лок → $hc (ожидали 429)"

# ---------- итог ----------
printf "\n${c_bold}Итого:${c_off} ${c_green}%d прошло${c_off}, " "$PASS"
if [ "$FAIL" -eq 0 ]; then
  printf "${c_green}0 провалено${c_off} ✓\n"; exit 0
else
  printf "${c_red}%d провалено${c_off}\n" "$FAIL"
  printf "${c_red}Провалившиеся:${c_off}\n"; for n in "${FAILED_NAMES[@]}"; do printf "  · %s\n" "$n"; done
  exit 1
fi
