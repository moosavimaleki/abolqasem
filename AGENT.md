# خلاصه مشکل و نیازمندی پروژه (Problem Statement)

## مشکل چیست؟
کاربر به طور پیوسته از ابزار خط فرمان Codex (نسخه TUI) برای برنامه‌نویسی و گفتگو با هوش مصنوعی استفاده می‌کند و زبان اصلی مکالمات او **فارسی** است. مشکل اساسی اینجاست که محیط ترمینال (Terminal/TUI) از رندرینگ صحیح متون راست‌چین (RTL) پشتیبانی نمی‌کند. در نتیجه، پاسخ‌های متنی هوش مصنوعی به هم ریخته نمایش داده می‌شوند و خواندن آن‌ها (مخصوصاً زمانی که شامل جدول‌های مارک‌داون یا ساختارهای متنی پیچیده باشند) بسیار آزاردهنده و دشوار است. این در حالی است که بلوک‌های کد و تغییرات گیت (git changes) چون چپ‌چین (LTR) هستند، بدون مشکل نمایش داده می‌شوند.

## خواسته و هدف
کاربر نیازمند راهکاری است تا بتواند در حین گفتگو با Codex، هر زمان که خواندن متن در ترمینال سخت شد، با یک دستور ساده و سریع (مثلاً یک دستور اسلش بومی یا پلاگین)، محتوای سشن فعلی را به محیطی خوانا منتقل کند. هدف این است که تاریخچه چت فعلی از فایل لاگ (rollout jsonl) خوانده شده و به یک فایل HTML با استایل‌های مناسب، فونت فارسی (مثل وزیرمتن)، پشتیبانی کامل از RTL و قالب‌بندی صحیح مارک‌داون (مانند جداول و بلوک‌های کد رنگی) تبدیل شود و به صورت خودکار در مرورگر وب باز شود.

## محدودیت‌ها و راهکار ایده‌آل
راهکار ارائه شده نباید هزینه‌بر باشد (مثلاً نباید برای انجام این تبدیل ساده، درخواستی به LLM ارسال شود و توکن مصرف کند). همچنین کاربر تمایل دارد این قابلیت به صورت یک **پلاگین مستقل** طراحی شود تا بتواند آن را در یک مخزن گیت‌هاب (مانند `ai-agent-manager`) منتشر کند و سایر کاربران نیز بتوانند بدون نیاز به دستکاری کدهای هسته (Core) سیستم Codex، از آن بهره‌مند شوند.

## دستورالعمل دائمی کیفیت کد و کنترل Bloat
در هر تغییر، قبل از تمام‌شده دانستن کار، bloat را بررسی کن: فایل‌های generated، build output، باینری‌ها، کد مرده، duplicate helperها، abstractionهای بی‌مصرف و dependencyهای اضافی نباید باقی بمانند. اگر چیزی قابل بازتولید است، آن را به عنوان سورس اصلی نگه ندار.

کد را همیشه صریح، شفاف، ساده و مستقیم بنویس. از پیچیده‌سازی، abstraction زودهنگام، wrapperهای بی‌دلیل و برنامه‌نویسی تدافعی غیرضروری پرهیز کن. فقط خطاها و حالت‌هایی را هندل کن که واقعاً در رفتار پروژه معنی دارند و برای کاربر یا سیستم قابل مشاهده‌اند.

## دستورالعمل دائمی توسعه Cross-platform
این پروژه فقط یک وب‌اپ ساده نیست؛ با فایل‌سیستم، cache، مسیرهای خانه کاربر، hooks، نصب باینری، Git، processها و تنظیمات ابزارهای مختلف کار می‌کند. بنابراین هر تغییری باید از ابتدا با فرض اجرای واقعی روی Linux، macOS و Windows طراحی شود.

- قبل از تغییر کدی که با path، فایل، cache، temp dir، home dir، process execution، shell command، Git یا installer سروکار دارد، تفاوت‌های Linux/macOS/Windows را صریح بررسی کن.
- مسیرها را با `filepath` و APIهای استاندارد سیستم‌عامل بساز، نه با concat کردن رشته‌ها یا `/` ثابت. فقط برای payloadهای وب/API از slash-normalized path استفاده کن.
- مسیرهای filesystem را در تست‌ها literal مقایسه نکن مگر مطمئن باشی canonical هستند. روی macOS مسیرهای temp ممکن است از `/var` به `/private/var` resolve شوند؛ در این موارد از `filepath.EvalSymlinks` یا normalization مناسب استفاده کن.
- برای cache و home dir به تفاوت‌های `XDG_CACHE_HOME`، `HOME`, `USERPROFILE`, `LOCALAPPDATA` و `os.UserCacheDir()` توجه کن. تست‌ها نباید روی cache واقعی runner یا سیستم توسعه‌دهنده بنویسند.
- کدهای نصب و hook باید command، permission، extension باینری (`.exe`) و shell متفاوت Windows/Unix را در نظر بگیرند. فرض نکن `sh`, `bash`, `chmod`, `install`, `tar` یا مسیرهای Unix همیشه موجودند.
- تستی که به asset ساخته‌شده، web build، binary release یا فایل generated نیاز دارد، یا خودش fixture را آماده کند یا در نبود fixture به شکل واضح skip شود. jobهای CI نباید به artifact باقی‌مانده از اجرای قبلی وابسته باشند.
- هر fix مرتبط با filesystem یا installer باید حداقل با `GOTOOLCHAIN=local go test ./...` و در صورت ارتباط با frontend با `npm run check` بررسی شود. اگر نمی‌توان یک OS را محلی اجرا کرد، تست‌ها را طوری بنویس که اختلاف‌های شناخته‌شده آن OS را پوشش دهند.

## دستورالعمل دائمی نام پروژه، Provider Runtime و CI
نام canonical پروژه `abolqasem` است. اگر Go package output، module path، import path، `ldflags` یا build script دوباره `ai-agent-manager/...` نشان داد، یعنی rename ناقص است. `ai-agent-manager` فقط در مسیرهای مهاجرت و compatibility مجاز است: legacy env مثل `AI_AGENT_MANAGER_*`، legacy localStorage key، legacy drag MIME type، تست تعمیر hookهای قدیمی، و constantهای `Legacy*`.

برای runtime providerها (`codex`, `claude`, `gemini`) هرگز command را با string خام و Unix-only تحلیل نکن. command ممکن است با absolute path، فاصله، پسوند `.exe`، یا مسیر Windows بیاید. برای تشخیص provider و قابلیت resume، همیشه basename را normalize کن: `filepath.Base`، lowercase، حذف پسوند `.exe`، سپس مقایسه با provider canonical.

تنظیمات مدل‌ها نباید فقط UI state بی‌اثر باشند. در معماری tmux-first، مدل پیش‌فرض فقط وقتی معتبر است که روی launch command همان provider اعمال شود، مثل `--model <id>`. تغییر مدل وسط سشن را فقط وقتی پیاده کن که برای همان provider با CLI واقعی و تست end-to-end ثابت شده باشد؛ slash command حدسی به tmux نفرست.

در تست‌های provider executable و tmux command:
- روی Windows هم `HOME` و هم `USERPROFILE` را در temp dir تست ست کن.
- fake executable را به صورت cross-platform بساز؛ shell script خام برای Windows کافی نیست.
- انتظار command کامل نباید به جداکننده مسیر یا پسوند سیستم‌عامل وابسته باشد. اگر هدف تست resume است، وجود subcommand/flagهای resume را assert کن و path را normalize کن.
- سناریوهای Windows را با تست صریح بپوشان: `codex.exe`, `claude.exe`, `gemini.exe` و configured path بدون `.exe` که باید fallback به `.exe` داشته باشد.

قبل از تمام‌شده دانستن هر تغییر مرتبط با provider runtime یا tmux، این موارد را چک کن:
- `go test ./internal/providers/providerexec ./internal/server -run 'ProviderCommand|TmuxCommand|Executable|Resume' -count=1`
- `go test ./...`
- خروجی `go test` باید با `abolqasem/...` شروع شود، نه `ai-agent-manager/...`.
