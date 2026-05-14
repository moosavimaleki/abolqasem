 

## Prompt برای اصلاح UI/UX History پیام‌های AI Agent در چت RTL برنامه‌نویسی

تو نقش یک **Senior Product Designer + Senior Frontend Engineer + UX Architect** را داری.
وظیفه تو اصلاح کامل UI/UX صفحه‌ی History پیام‌های یک AI Agent است.

این صفحه برای نمایش تاریخچه‌ی پیام‌های Agent، پیام‌های کاربر، پاسخ‌های Assistant، tool call ها، metadata، token usage، task events و raw payload ها استفاده می‌شود.

هدف نهایی این است که تجربه کاربری از حالت **خام، لاگ‌محور و گیج‌کننده** تبدیل شود به یک تجربه شبیه **ChatGPT Conversation History**؛ یعنی خوانا، تمیز، حرفه‌ای، قابل debug، مناسب فارسی/RTL و مناسب برنامه‌نویسی.

### مسئله فعلی

در UI فعلی مشکلات جدی وجود دارد:

پیام‌ها خام نمایش داده می‌شوند. Markdown به HTML زیبا تبدیل نشده. جدول‌ها، code block ها، heading ها، list ها و blockquote ها ظاهر مناسب ندارند. tool call ها و payload ها وسط مکالمه ریخته شده‌اند و مسیر اصلی خواندن را خراب کرده‌اند. پیام‌های کاربر و پیام‌های Assistant تقریباً هم‌وزن دیده می‌شوند، درحالی‌که پیام کاربر بیشتر نقش context دارد و نباید توجه اصلی را بگیرد. متن فارسی، JSON، کد، timestamp، ID و base64 از نظر RTL/LTR با هم قاطی شده‌اند. نتیجه این شده که صفحه بیشتر شبیه raw log viewer است، نه history قابل‌خواندن یک AI Agent.

### هدف طراحی

صفحه باید تبدیل شود به:

**AI Agent Conversation History for RTL Programming Chat**

یعنی:

پیام‌های اصلی مثل یک چت تمیز دیده شوند.
پاسخ Assistant با Markdown زیبا و readable render شود.
پیام کاربر کوچک‌تر، کم‌رنگ‌تر و compact باشد.
tool call ها پیش‌فرض collapsed باشند.
Raw JSON فقط برای debug قابل باز شدن باشد.
کد، JSON، tool call، ID، timestamp و payload های فنی همیشه LTR باشند.
متن فارسی و توضیحات انسانی RTL باشند.
حس کلی صفحه باید شبیه تجربه‌ی خوب ChatGPT باشد: خوانا، خلوت، با hierarchy واضح، بدون ریختن همه‌چیز روی سر کاربر.

---

## اصول قطعی طراحی

### 1. Conversation اولویت دارد، Log نه

مسیر اصلی صفحه باید فقط شامل این‌ها باشد:

User Message
Assistant Message
خلاصه قابل‌فهم از status یا event های مهم

Tool call، raw payload، encrypted content، token event و metadata نباید در مسیر اصلی خواندن مزاحمت ایجاد کنند. این‌ها باید در لایه‌ی debug قرار بگیرند.

---

### 2. پیام Assistant باید Markdown-rendered باشد

هر پاسخ Assistant باید از Markdown خام تبدیل شود به HTML زیبا.

پشتیبانی کامل لازم است برای:

Heading ها
Paragraph ها
Bold و italic
Numbered list
Bullet list
Nested list
Inline code
Code block
Table
Blockquote
Links
Horizontal rule
Mixed Persian/English text

ظاهر Markdown باید شبیه یک document تمیز داخل chat bubble باشد، نه متن خام.

Heading ها باید hierarchy واضح داشته باشند.
پاراگراف‌ها line-height مناسب داشته باشند.
list ها باید فاصله و indentation درست داشته باشند.
blockquote باید با visual treatment جدا نمایش داده شود.
inline code باید badge کوچک و خوانا باشد.
code block باید کارت جدا، LTR، monospace و قابل کپی باشد.
جدول باید داخل wrapper زیبا و responsive باشد.

---

### 3. جدول‌ها UI اختصاصی لازم دارند

Markdown table نباید به شکل متن خام یا جدول زشت دیده شود.

هر جدول باید:

ظاهر card-like داشته باشد.
header متفاوت داشته باشد.
border و padding مناسب داشته باشد.
در عرض کم horizontal scroll داشته باشد.
در RTL کلی صفحه خراب نشود.
محتوای عددی یا فنی در صورت نیاز LTR باشد.
در موبایل یا عرض کم نشکند.

هدف این است که جدول در پاسخ‌های فنی، مقایسه‌ای و تحلیلی کاملاً خوانا باشد.

---

### 4. Code block ها باید تجربه‌ی برنامه‌نویسی درست داشته باشند

هر code block باید:

LTR باشد.
text-align left داشته باشد.
monospace باشد.
background جدا داشته باشد.
language label داشته باشد، اگر زبان مشخص است.
دکمه copy داشته باشد.
overflow-x کنترل‌شده داشته باشد.
از متن فارسی اطرافش جدا باشد.
در dark mode خوانا باشد.

کد نباید داخل جهت RTL کشیده یا خراب شود.

---

### 5. پیام کاربر باید کم‌اهمیت‌تر از پاسخ Agent دیده شود

در این UI، تمرکز اصلی روی خروجی Agent است. پیام کاربر context است.

پس User Message باید:

کوچک‌تر از Assistant Message باشد.
رنگ متفاوت داشته باشد.
فضای کمتری بگیرد.
compact باشد.
در صورت طولانی بودن قابل collapse باشد.
از نظر بصری مزاحم پاسخ Assistant نباشد.

اما همچنان باید قابل خواندن و قابل تشخیص باشد.

حس درست:

User Message = سؤال/فرمان/کانتکست
Assistant Message = خروجی اصلی قابل مطالعه

---

### 6. Tool call ها باید پیش‌فرض collapsed باشند

Tool call ها مسیر اصلی خواندن نیستند. آن‌ها برای debug هستند.

هر tool call باید به صورت یک آیتم بسته نمایش داده شود با اطلاعات خلاصه:

نام tool
وضعیت: running / completed / failed
مدت اجرا
زمان اجرا
خلاصه کوتاه input/output در صورت مفید بودن
دکمه expand/collapse

وقتی باز شد، داخل آن باید بخش‌های جدا داشته باشد:

Input
Output
Error
Raw JSON
Metadata

همه محتوای داخل tool call باید LTR باشد، چون معمولاً JSON، key، value، ID، path، request، response یا stack trace است.

Tool call نباید مثل پیام Assistant نمایش داده شود.
Tool call نباید فضای اصلی مکالمه را اشغال کند.
Tool call نباید خام و بازشده به کاربر تحمیل شود.

---

### 7. Raw JSON فقط در حالت Debug دیده شود

هیچ raw JSON، raw payload، encrypted content، base64، event object یا metadata حجیم نباید به صورت پیش‌فرض در صفحه باز باشد.

برای هر raw data باید رفتار زیر وجود داشته باشد:

خلاصه انسانی در UI اصلی
دکمه View raw
دکمه Copy
حالت collapse/expand
نمایش LTR و monospace
امکان scroll افقی
truncate برای مقادیر خیلی طولانی

مثلاً به جای نمایش کامل encrypted_content، فقط نشان بده:

Encrypted content available
Length: X characters
View raw
Copy

---

### 8. مدیریت RTL/LTR باید هوشمند و دقیق باشد

صفحه کلی برای فارسی باید RTL باشد.

اما این موارد همیشه باید LTR باشند:

Code block
Inline code
JSON
Tool call input/output
Raw payload
Timestamps
IDs
UUID
URLs
File paths
Terminal commands
Stack traces
Token names
Model names
Base64/encrypted strings
Numbers ترکیبی با key های انگلیسی

متن فارسی، توضیحات انسانی، عنوان‌ها و پاسخ‌های عادی باید RTL باشند.

برای محتوای mixed، باید direction در سطح block کنترل شود، نه اینکه کل صفحه یک direction ثابت داشته باشد و همه چیز را خراب کند.

هدف: همان حسی که در ChatGPT وجود دارد؛ فارسی راست‌چین و خوانا، کد و JSON چپ‌چین و سالم.

---

### 9. Hierarchy بصری باید واضح باشد

کاربر باید با یک نگاه بفهمد:

کدام پیام از کاربر است؟
کدام پاسخ Assistant است؟
کدام بخش tool call است؟
کدام بخش metadata است؟
کدام event خطا دارد؟
کدام task کامل شده؟
کدام بخش قابل باز شدن است؟
کدام بخش خروجی اصلی است؟

برای این کار از visual hierarchy استفاده کن:

اندازه فونت متفاوت
رنگ متفاوت
background متفاوت
spacing متفاوت
badge برای status
border سبک
card layout
collapse sections
metadata کوچک و کم‌رنگ
error با treatment مشخص
success/completed با treatment آرام و واضح

---

### 10. صفحه باید ChatGPT-like باشد، نه DevTools-like

این UI برای history پیام‌های AI Agent است، نه صرفاً console log.

حس مورد انتظار:

تمیز
خلوت
خوانا
متمرکز روی conversation
مناسب long-form answer
مناسب پاسخ‌های فنی
مناسب Markdown سنگین
مناسب فارسی
مناسب debug در صورت نیاز
نه شلوغ، نه خام، نه شبیه dump دیتابیس

از ChatGPT الهام بگیر:

پاسخ Assistant فضای کافی دارد.
Markdown خوب render می‌شود.
کدها جدا هستند.
متن کاربر از متن Assistant قابل تشخیص است.
جزئیات فنی مزاحم خواندن نمی‌شوند.
کاربر اول جواب را می‌بیند، بعد اگر خواست debug را باز می‌کند.

---

## ساختار پیشنهادی صفحه

صفحه بهتر است این بخش‌ها را داشته باشد:

Header جلسه
لیست session ها
ناحیه اصلی conversation
پنل اختیاری details/debug
فیلتر event ها
جستجو داخل session
دکمه jump to latest
کنترل auto-scroll

Header جلسه باید نشان دهد:

نام session
status
زمان شروع
آخرین activity
تعداد پیام‌ها
تعداد tool call ها
مدت اجرا
وضعیت کلی token/quota اگر وجود دارد

اما این metadata نباید مسیر اصلی خواندن را شلوغ کند.

---

## طراحی پیشنهادی برای هر Message

### User Message

ویژگی‌ها:

کوچک‌تر
compact
رنگ متفاوت
opacity کمتر
قابل collapse اگر طولانی بود
label کوچک “User”
timestamp کوچک
بدون Markdown سنگین مگر لازم باشد

هدف: context بدهد، نه اینکه مرکز توجه شود.

### Assistant Message

ویژگی‌ها:

عرض مناسب برای خواندن
Markdown کامل
spacing عالی
typography قوی
کد و جدول زیبا
metadata کوچک در بالا یا پایین
قابل copy
در صورت streaming، حالت loading/typing مناسب
اگر خطا داشت، error state واضح

هدف: خروجی اصلی و readable.

### Tool Call

ویژگی‌ها:

collapsed by default
خلاصه یک‌خطی یا دوخطی
LTR
monospace برای raw بخش‌ها
status badge
duration
expand برای input/output/raw
copy برای JSON
error state واضح
عدم مزاحمت برای conversation

### System/Event Message

ویژگی‌ها:

خیلی compact
کم‌رنگ
فقط برای event های مهم
قابل فیلتر شدن
نه در قالب message اصلی
در صورت زیاد بودن، داخل timeline/debug tab برود

---

## رفتار با داده‌های طولانی

هر مقدار خیلی طولانی باید truncate شود.

مصادیق:

encrypted_content
base64
large JSON
long stack trace
long tool output
large markdown raw
large array/object
logs

رفتار مطلوب:

نمایش خلاصه
نمایش طول محتوا
دکمه expand
دکمه copy
در صورت باز شدن، scroll داخلی
نه بزرگ‌کردن بی‌نهایت ارتفاع صفحه

---

## Empty State و Loading State

اگر session خالی است، پیام واضح بده:

هنوز پیامی در این session وجود ندارد.

اگر Agent در حال اجراست:

نمایش وضعیت running
نمایش آخرین event
نمایش typing/processing indicator
نمایش tool call جاری به صورت collapsed اما با status running

اگر خطا رخ داده:

نمایش error card خوانا
خلاصه انسانی خطا
جزئیات فنی داخل collapsible debug

---

## Accessibility

Accordion و collapsible ها باید keyboard-friendly باشند.

کاربر باید با کیبورد بتواند:

بین پیام‌ها حرکت کند
tool call را باز و بسته کند
raw JSON را باز کند
copy کند
روی لینک‌ها حرکت کند

Focus state باید واضح باشد.
کنتراست متن در dark mode باید کافی باشد.
دکمه‌ها باید label واضح داشته باشند.
آیکون بدون متن کافی نیست.
collapse state باید قابل فهم باشد.

---

## Responsive Behavior

در دسکتاپ:

Session list در کنار صفحه
Conversation در مرکز
Details/debug panel در سمت دیگر یا drawer

در عرض کم:

Session list به drawer تبدیل شود
Details/debug به bottom sheet یا modal تبدیل شود
Conversation اولویت اصلی باشد
جدول‌ها horizontal scroll داشته باشند
کدها overflow-x داشته باشند

---

## فیلترها و کنترل‌ها

امکانات مفید:

نمایش فقط messages
نمایش tool calls
نمایش errors
نمایش raw events
جستجو در session
jump to latest
auto-scroll on/off
collapse all tool calls
expand failed tool calls only
copy full assistant answer
copy raw session
copy selected event

---

## Acceptance Criteria

این موارد باید پاس شوند:

1. پیام‌های Assistant با Markdown واقعی و زیبا render شوند.
2. Markdown خام نباید در UI اصلی دیده شود، مگر در حالت debug.
3. Heading، paragraph، list، table، code block، inline code، blockquote و link ظاهر اختصاصی داشته باشند.
4. جدول‌ها responsive، خوانا و دارای scroll افقی باشند.
5. Code block ها LTR، monospace، دارای copy و visually separated باشند.
6. پیام‌های User کوچک‌تر، compact تر و با رنگ متفاوت از Assistant باشند.
7. پیام User نباید با پاسخ Assistant هم‌وزن باشد.
8. Tool call ها پیش‌فرض collapsed باشند.
9. Tool call input/output/raw همگی LTR باشند.
10. Raw JSON فقط با View Raw یا Expand دیده شود.
11. داده‌های طولانی truncate شوند و صفحه را نشکنند.
12. متن فارسی RTL و خوانا باشد.
13. کد، JSON، ID، timestamp، URL و path همیشه LTR باشند.
14. Error state، completed state و running state از هم قابل تشخیص باشند.
15. UI نباید شبیه dump لاگ باشد؛ باید شبیه conversation history حرفه‌ای باشد.
16. در dark mode خوانایی و contrast مناسب باشد.
17. صفحه در دسکتاپ و موبایل usable باشد.
18. collapse/expand ها keyboard accessible باشند.
19. جزئیات فنی برای debug در دسترس باشند، اما مزاحم خواندن نباشند.
20. تجربه نهایی باید حس ChatGPT-like برای چت RTL برنامه‌نویسی بدهد.

---

## خروجی مورد انتظار از تو

ابتدا UI فعلی را از نظر structure و hierarchy بررسی کن.
سپس یک طراحی اصلاح‌شده برای کامپوننت‌ها و layout ارائه بده.
بعد implementation را انجام بده.
در پایان توضیح بده دقیقاً کدام مشکلات حل شده‌اند.

در طراحی نهایی، اولویت با این‌هاست:

خوانایی پاسخ Assistant
Markdown rendering حرفه‌ای
RTL/LTR درست
Tool call های collapsed
Raw JSON فقط برای debug
تجربه شبیه ChatGPT
مناسب چت‌های فنی فارسی/برنامه‌نویسی

نکته مهم:
این پروژه قرار نیست فقط داده را نمایش دهد. قرار است history یک AI Agent را به شکلی نشان دهد که انسان بتواند سریع بفهمد چه پرسیده شده، Agent چه جواب داده، چه tool هایی استفاده شده، و در صورت نیاز بتواند وارد debug شود.

جمع‌بندی هدف:

**Conversation برای خواندن.
Tool call برای debug.
Raw JSON برای مواقع ضروری.
RTL برای متن انسانی فارسی.
LTR برای کد و داده فنی.**

[1]: https://www.w3.org/International/questions/qa-html-dir.en.html?utm_source=chatgpt.com "Structural markup and right-to-left text in HTML"
