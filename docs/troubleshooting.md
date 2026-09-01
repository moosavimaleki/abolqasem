# عیب‌یابی Codex Manager

- **Gateway آماده نمی‌شود:** در Settings وضعیت gateway و Crash count را ببینید،
  سپس Refresh یا restart را اجرا کنید. port دیگری روی loopback نباید اشغال باشد.
- **کد ورود منقضی شد:** dialog را ببندید، Login جدید بگیرید و URL را در همان
  مرورگری که کد را نشان می‌دهد باز کنید.
- **مدل پیدا نمی‌شود:** در Custom Provider ابتدا Test/Discover را بزنید؛ برای
  provider داخلی mapping نسازید و base URL را با `/v1` درست وارد کنید.
- **session Chrome خوانده نمی‌شود:** Chrome را ببندید یا اجازهٔ خواندن profile
  را بدهید. Abolqasem فایل cookie را فقط به snapshot موقت کپی می‌کند.
- **تاریخچه کند است:** range نمودار را کم کنید یا sessionهای قدیمی را archive
  کنید؛ API و chart عمداً history را محدود می‌کنند.
