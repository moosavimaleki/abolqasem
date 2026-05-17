# AI Agent Manager Product UX

## خلاصه

`AI Agent Manager` نباید از یک viewer ساده به یک IDE سنگین تبدیل شود. محصول باید یک فضای محلی برای دیدن، خواندن، دنبال کردن و در صورت امکان ادامه دادن sessionهای agent باشد.

مدل ذهنی پیشنهادی:

```txt
اینجا همه نشست‌های agentهایم را می‌بینم.
اگر agent زنده یا قابل ادامه دادن باشد، همین‌جا ادامه می‌دهم.
اگر فقط history داشته باشم، همان را راحت و خوانا می‌خوانم.
```

پس محصول یک toggle بزرگ بین `Viewer` و `Web UI` لازم ندارد. تجربه باید یکپارچه باشد و UI خودش بر اساس قابلیت هر session تصمیم بگیرد چه کاری ممکن است.

## اصول طراحی

1. پیچیدگی runtime به کاربر منتقل نشود.
2. خواندن history همچنان قابلیت اصلی و همیشه قابل اعتماد باشد.
3. ارسال پیام فقط وقتی فعال شود که session واقعاً قابل کنترل یا قابل resume باشد.
4. hookها نباید تمرکز کاربر را بدزدند وقتی کاربر درگیر یک session است.
5. هم‌زمانی agentها باید بر اساس workspace کنترل شود، نه فقط بر اساس session.
6. web UI نباید مستقیماً agentها را expose کند؛ backend محلی باید کنترل، امنیت و approval را مدیریت کند.
7. اگر agent نمی‌تواند کنترل شود، UI باید آن را آرام و دقیق نشان دهد، نه با خطای فنی.

## محصول از دید کاربر

کاربر یک صفحه اصلی دارد:

```txt
Sidebar: پروژه‌ها و sessionهای اخیر
Main: مکالمه، reader، file preview، live output
Bottom: composer context-aware
Top: نام session، نام پروژه، status و actions
```

کاربر لازم نیست بداند الان در `viewer mode` است یا `web ui mode`. صفحه همیشه session را نشان می‌دهد. فقط composer و actionها بسته به وضعیت فعال یا غیرفعال می‌شوند.

## وضعیت‌های session

هر session از نظر UI می‌تواند این وضعیت‌ها را داشته باشد:

| وضعیت | معنی | رفتار UI |
| --- | --- | --- |
| `Archived` | فقط transcript/history داریم | composer غیرفعال، reader/search/file preview فعال |
| `Attachable` | runtime یا thread قابل resume است | دکمه `Resume` یا composer آماده فعال‌سازی |
| `Live` | agent زنده است و event می‌دهد | live updates، status، composer وابسته به control |
| `Controlling` | web کنترل نوشتن دارد | composer فعال، approvalها در web |
| `Busy` | agent در حال کار است | composer بسته به agent می‌تواند `Steer` یا queue نشان دهد |
| `Blocked` | agent منتظر approval یا input است | approval panel برجسته می‌شود |
| `Unavailable` | bridge یا runtime در دسترس نیست | composer disabled با علت کوتاه |

این وضعیت‌ها نباید به صورت modeهای بزرگ نمایش داده شوند. فقط باید در UI به شکل subtle state، badge، disabled state و action مناسب دیده شوند.

## Composer

composer همیشه در جای ثابت پایین مکالمه باشد، اما state آن تغییر کند:

| state | متن/رفتار |
| --- | --- |
| session قابل کنترل است | input فعال، ارسال پیام |
| session قابل resume است | `Resume and continue` |
| agent busy است | اگر agent پشتیبانی کند `Steer`، وگرنه queue/disabled |
| فقط history است | composer disabled |
| runtime نصب نیست | action برای راه‌اندازی یا نصب bridge |
| approval لازم است | composer عقب می‌رود و approval panel اولویت می‌گیرد |

هدف این است که کاربر حس کند «همان‌جا ادامه می‌دهم»، نه اینکه وارد یک محصول دیگر شود.

## رفتار hookها

hookها هنوز مهم هستند، چون ابزارهای TUI و CLI از بیرون sessionها را update می‌کنند. اما سیاست navigation باید عوض شود.

قانون اولویت:

```txt
manual user choice > active web control > user typing/focus > hook updates
```

رفتار پیشنهادی:

| شرایط | رفتار hook |
| --- | --- |
| کاربر idle است و session خاصی را کنترل نمی‌کند | می‌تواند auto-follow یا notice بدهد |
| hook برای همین session فعلی است | همان session refresh شود |
| کاربر در composer تایپ می‌کند | focus جابه‌جا نشود، فقط badge/update |
| web کنترل یک session را دارد | hook نباید session فعلی را عوض کند |
| hook برای session دیگر است | sidebar badge، toast کم‌مزاحمت |

به این ترتیب viewer بودن برنامه حفظ می‌شود، ولی وقتی برنامه در حال کنترل agent است، hookها رفتار مزاحم ندارند.

## کنترل و مالکیت

برای هر session باید بین دیدن و کنترل فرق بگذاریم:

| مفهوم | معنی |
| --- | --- |
| `observe` | فقط دیدن history و live updates |
| `control` | ارسال پیام، approve، interrupt، steer |
| `resume` | زنده کردن session قدیمی و ادامه دادن |
| `take over` | گرفتن کنترل از یک client دیگر |

کاربر نباید درگیر واژه‌های فنی شود. در UI این‌ها می‌توانند این‌طور دیده شوند:

| حالت فنی | label پیشنهادی |
| --- | --- |
| resume | `Continue` |
| control attach | `Continue here` |
| force control | `Take over` |
| observe only | فقط نمایش history/live |

برای یک session مشخص فقط یک writer/controller مجاز باشد. چند observer مشکلی ندارند.

## چند agent هم‌زمان

وب می‌تواند چند session فعال داشته باشد که LLM واقعاً روی آن‌ها کار می‌کند. اما محدودیت اصلی workspace است.

قانون پیشنهادی برای MVP:

```txt
هر project root فقط یک active writer داشته باشد.
چند session می‌توانند مشاهده شوند.
چند session می‌توانند روی پروژه‌های مختلف هم‌زمان کار کنند.
```

اگر کاربر بخواهد session دوم را روی همان پروژه شروع کند:

| گزینه | معنی |
| --- | --- |
| `Watch only` | فقط مشاهده session |
| `Queue` | بعد از پایان writer فعلی شروع شود |
| `Take over` | کنترل writer فعلی منتقل شود |
| `Run separately` | در نسخه آینده با git worktree جدا |

نسخه پیشرفته می‌تواند برای parallel agentها از `git worktree` استفاده کند:

```txt
same repo
  -> worktree/session-a
  -> worktree/session-b
  -> compare/merge/review
```

این برای MVP لازم نیست، ولی باید در طراحی آینده جا داشته باشد.

## پروژه‌ها و sessionها

sidebar فعلی باید مفهوم `Recent Sessions` را نگه دارد، اما presentation باید پروژه‌محورتر شود.

هر item بهتر است این‌ها را نشان دهد:

```txt
Project name
Session title
Agent badge
Live/Archived/Busy status
Unread/update badge
```

نام پروژه از نام پوشه می‌آید و قابل تغییر نیست. نام session از اولین پیام مشتق می‌شود و کاربر می‌تواند آن را ویرایش کند.

کلیک روی پروژه باید لیست sessionهای همان پروژه را باز کند. این کمک می‌کند کاربر بین conversationهای یک repo گم نشود.

## Header

header session باید دو نقش داشته باشد:

1. هویت session فعلی را نشان دهد.
2. actions مهم را بدون شلوغی در دسترس بگذارد.

چیدمان پیشنهادی:

```txt
[Project pill]          [Session title editable]          [status/actions]
```

`Project pill` باید واضح کند که پروژه است و sessionهای دیگری دارد. `Session title` باید editable بودن را ظریف نشان دهد، نه مثل input دائمی.

## Approval UX

وقتی محصول interactive شود، approval مهم‌ترین سطح امنیتی UI است.

approval نباید toast کوچک باشد. باید یک panel مشخص داشته باشد که نشان دهد:

```txt
agent
session
project
requested action
command/file diff
risk level
allow once / deny / details
```

برای فایل edit، diff مهم‌تر از متن توضیحی است. برای shell command، command و working directory باید واضح باشند.

اگر چند session هم‌زمان approval بخواهند:

- approval session فعلی در main area نمایش داده شود.
- approval sessionهای دیگر badge بگیرند.
- کاربر با کلیک به همان session برود.

## امنیت

وقتی composer فعال می‌شود، محصول دیگر فقط viewer نیست. backend می‌تواند باعث اجرای command و edit فایل شود.

حداقل سیاست امنیتی:

1. فقط روی `127.0.0.1`.
2. workspace allowlist.
3. file preview محدود به مسیرهای مجاز.
4. control bridge بدون auth عمومی expose نشود.
5. command/edit فقط از مسیر approval agent یا policy محلی عبور کند.
6. لینک‌های فایل فقط برای مسیرهای local امن resolve شوند.

## تنظیمات برنامه

کنار `reader settings` باید یک بخش تنظیمات کلی برای خود برنامه وجود داشته باشد. این تنظیمات نباید حس configuration فنی سنگین بدهند؛ باید شبیه یک control center کوچک برای رفتار viewer، hookها، server و agent bridge باشند.

ورودی پیشنهادی:

```txt
Top bar settings icon
  -> Settings modal / panel
```

ساختار پیشنهادی:

| بخش | تنظیمات |
| --- | --- |
| `Notifications` | hook notifications، toastها، unread badge |
| `Session following` | auto-follow hook updates، فقط notice دادن، خاموش |
| `Event sources` | hooks، runtime events، filesystem discovery |
| `Agent control` | اجازه resume/continue، take over confirmation، writer lock behavior |
| `Server` | restart server، open logs، show base URL، startup mode |
| `Security` | workspace allowlist، local-only status، file link policy |
| `Reader` | لینک به reader settings موجود |

تنظیمات مهم برای hookها:

| setting | معنی |
| --- | --- |
| `Hook updates` | hookها sessionهای TUI/CLI را به برنامه اطلاع بدهند |
| `Auto-follow active hook session` | وقتی کاربر idle است، session جدید hook شده باز شود |
| `Show hook notices only` | جابه‌جایی خودکار خاموش، فقط notice/badge |
| `Ignore hook navigation while typing` | وقتی composer یا search focus دارد، focus حفظ شود |
| `Background discovery interval` | walk/polling دیسک برای recovery |

پیشنهاد default:

```txt
Hook updates: on
Auto-follow: on only when idle
Hook notices: on
Ignore while typing/controlling: on
Filesystem discovery: on
Runtime events: on when available
```

تنظیمات server باید عملیات‌های نگهداری رایج را در UI بیاورد:

| action | رفتار |
| --- | --- |
| `Restart server` | همان کاری که CLI restart انجام می‌دهد |
| `Reload sessions` | discovery فوری و refresh sidebar |
| `Open logs` | باز کردن log/diagnostics در modal یا route |
| `Copy local URL` | کپی base URL فعلی |
| `Check hooks` | وضعیت hookهای Codex/Gemini/Claude |

نکته UX: این بخش نباید جای نصب پیچیده شود. نصب و trust همچنان باید از installer ساده بماند. تنظیمات برنامه فقط برای کنترل رفتار بعد از نصب است.

## Agent Adapter Layer

برای interactive شدن، backend باید adapter داشته باشد:

```txt
Web frontend
  -> local backend
  -> agent adapter
  -> Codex app-server / Claude SDK / Gemini ACP
```

هر adapter باید capability خود را اعلام کند:

```txt
canList
canStart
canSend
canSteer
canInterrupt
canApprove
supportsLiveEvents
supportsMultipleRuns
```

UI نباید برای هر agent جدا طراحی شود. UI باید از capabilityها تغذیه شود.

## Empty و Error States

پیام‌های خالی باید عملی و کوتاه باشند:

| شرایط | پیام |
| --- | --- |
| هیچ session نیست | `No sessions yet` |
| جستجو نتیجه ندارد | `No matching sessions` |
| session فقط history است | `This session is read-only` |
| runtime در دسترس نیست | `Agent runtime is not available` |
| workspace locked است | `Another session is changing this project` |

در متن UI از توضیح طولانی پرهیز شود. detail می‌تواند در tooltip یا expandable details باشد.

## MVP پیشنهادی

MVP نباید از روز اول multi-agent کامل باشد. مسیر سالم:

1. حفظ viewer فعلی و session discovery.
2. اضافه کردن capability model به sessionها.
3. اضافه کردن composer context-aware، ابتدا فقط disabled/read-only.
4. adapter اول برای Codex.
5. `Continue` برای sessionهای قابل resume.
6. `New Session` برای Codex.
7. event streaming live برای sessionهای web-created.
8. approval UI پایه.
9. one writer per workspace.
10. settings panel کلی برای hookها، auto-follow، event sources و server actions.
11. بعد از تثبیت، Gemini ACP و Claude SDK/headless.

## تصمیم‌های باز

این موارد قبل از implementation باید قطعی شوند:

1. session قابل resume را چطور تشخیص می‌دهیم؟
2. mapping بین transcript session و runtime thread کجا persist می‌شود؟
3. وقتی TUI و web هم‌زمان یک session را می‌بینند، transfer control چطور اعلام می‌شود؟
4. اگر agent busy باشد، پیام جدید queue می‌شود یا steer؟
5. workspace root را چگونه تشخیص می‌دهیم؟
6. writer lock در سطح repo است یا در سطح directory؟
7. approvalهای agentهای مختلف در UI واحد چگونه normalize می‌شوند؟
8. web-created sessionها آیا حتماً transcript hook-compatible تولید می‌کنند؟
9. تنظیمات کلی برنامه کجا persist می‌شود و چطور با CLI config sync می‌شود؟
10. خاموش کردن hook updates فقط رفتار UI را خاموش می‌کند یا hook نصب‌شده را هم disable می‌کند؟

## تعریف موفقیت

نسخه خوب این محصول وقتی موفق است که:

1. کاربر همچنان آن را برای خواندن sessionها باز کند، حتی اگر هیچ interactive feature استفاده نکند.
2. وقتی session قابل ادامه دادن است، ادامه دادن طبیعی و بی‌اصطکاک باشد.
3. hookها دیگر حس پرت شدن ناگهانی ایجاد نکنند.
4. کاربر بفهمد کدام agent در حال کار است و روی کدام پروژه.
5. هم‌زمانی agentها باعث خراب شدن workspace نشود.
6. approvalها واضح، قابل اعتماد و بدون ابهام باشند.

## جمع‌بندی طراحی

محصول نهایی باید این باشد:

```txt
AI Agent Manager
  = session viewer
  + reader for hard-to-read agent output
  + project/session explorer
  + optional live control when available
  + safe local bridge for agent runtimes
```

نه یک viewer جدا، نه یک chat UI جدا. یک سطح واحد برای کار با sessionهای agent که همیشه خواندن را خوب انجام می‌دهد و هر وقت runtime اجازه داد، ادامه دادن را هم ممکن می‌کند.
