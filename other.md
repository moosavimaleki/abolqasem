ما چی؟

ما زیرساخت fork را داریم، اما فرمان chat.fork فعلی کامل وصل نشده.

در کد ما:

workspaceForkChat فقط chat جدید می‌سازد.
title را (... Fork) می‌کند.
پیام‌های قبلی را کپی می‌کند.
ولی pendingForkSessionToken را ست نمی‌کند.
provider/sessionToken را هم منتقل نمی‌کند.
یعنی در UI شبیه fork دیده می‌شود، اما از نظر native agent session فعلاً fork واقعی نیست.

به من بگو ما این ها رو پیاده سازی کردمی؟
اینها:
projects.jsonl: باز/حذف/rename پروژه‌ها
chats.jsonl: ساخت، rename، archive/delete، provider، plan mode
turns.jsonl: شروع/اتمام/خطا/کنسل turn و sessionToken
queued-messages.jsonl: پیام‌های صف‌شده
snapshot.json: snapshot کامپکت‌شده
transcripts/<chatId>.jsonl: transcript هر chat به صورت جداگانه
نکته مهم: Abolqasem خود session کامل agent را کپی نمی‌کند؛ فقط sessionToken را ذخیره می‌کند تا Claude/Codex بتوانند resume کنند. برای fork هم pendingForkSessionToken دارد. این schema در events.ts آمده.
snapshot.json	پیاده شده. compact snapshot داریم، ولی فقط projects، chats و queuedMessages را نگه می‌دارد.
transcripts/<chatId>.jsonl	پیاده نشده. این تفاوت مهم است. ما همه transcriptها را در messages.jsonl به صورت eventهای message_appended نگه می‌داریم و موقع خواندن بر اساس chatId فیلتر می‌کنیم.
فقط ذخیره sessionToken برای resume	برای حالت عادی پیاده شده. session کامل agent را ذخیره نمی‌کنیم، sessionToken را ذخیره می‌کنیم.
pendingForkSessionToken	schema و read-model و starter آن را داریم، اما command فعلی chat.fork آن را ست نمی‌کند. پس fork واقعی کامل نیست.


ایا بهتر نیست که سشن همه رو قتی سکن میکنیم متا دیتای مربوط به abolqasem اون رو هم بسازیم که مجبور نشیم legacy کنیم
فقط با یه فلگش مشخص بشه که این سشن native هست و یه سری متا دیتا رو نداره و کاربر میتونه ست کنه یا اگر میشه اتوماتیک ست بشه باید بشه

چه متا دیتایی رو نداریم؟
ایا روش من مناسب است؟


اگر بخواهیم سشن TUI را از همان scan به Abolqasem-style chat تبدیل کنیم، این metadataهای Abolqasem را از native session نداریم یا ناقص داریم:

1. chatId
   Native session فقط sessionId دارد. باید deterministic chatId بسازیم.
2. projectId
   Native فقط cwd دارد. باید project را از cwd بسازیم یا به project موجود map کنیم.
3. title
   Native title رسمی Abolqasem ندارد. باید از اولین user prompt یا basename پروژه بسازیم.
4. createdAt
   گاهی از timestamp فایل یا session_meta.timestamp قابل حدس است، ولی همیشه دقیق نیست.
5. updatedAt
   از mtime فایل یا session_index.updated_at قابل حدس است.
6. deletedAt
   native ندارد. این فقط UI metadata داخلی ماست.
7. archivedAt
   native ندارد. فقط داخلی ماست.
8. unread
   native ندارد. فقط داخلی ماست.
9. provider
   از مسیر scan معلوم می‌شود: codex، claude، gemini.
10. planMode
    native معمولاً ندارد یا مطمئن نیست. باید default باشد.
11. sessionToken
    داریم: همان native session id/thread id.
12. pendingForkSessionToken
    native ندارد. فقط وقتی در UI fork کنیم ساخته می‌شود.
13. hasMessages
    از transcript قابل محاسبه است.
14. lastMessageAt
    از آخرین پیام transcript یا mtime قابل حدس است.
15. lastTurnOutcome
    native معمولاً مستقیم ندارد. شاید از آخرین result/error قابل استخراج باشد، ولی قابل اعتماد کامل نیست.
16. model
    در ChatRecord اصلی Abolqasem ذخیره نمی‌شود؛ بیشتر در composer/settings و system_init transcript می‌آید. برای native TUI ممکن است از transcript قابل استخراج باشد.
17. modelOptions / effort
    در ChatRecord اصلی Abolqasem نیست؛ برای queued/composer است. از native ممکن است ناقص یا نامعلوم باشد.
18. transcripts/<chatId>.jsonl
    native transcript در مسیر خودش است، مثلا ~/.codex/sessions/...jsonl. برای Abolqasem-style storage باید تصمیم بگیریم کپی کنیم یا فقط link کنیم.






