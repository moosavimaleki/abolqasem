use crate::Binding;
use std::{
    collections::HashMap,
    fs, io,
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

pub struct BindingStore {
    path: PathBuf,
    values: HashMap<String, Binding>,
}

impl BindingStore {
    pub fn load(path: PathBuf) -> Self {
        let values = fs::read(&path)
            .ok()
            .and_then(|bytes| serde_json::from_slice::<HashMap<String, Binding>>(&bytes).ok())
            .unwrap_or_default();
        Self { path, values }
    }

    pub fn account(&self, conversation: &str) -> Option<&str> {
        self.values
            .get(conversation)
            .map(|binding| binding.account.as_str())
    }

    pub fn bind(&mut self, conversation: String, account: String, reason: &str) -> io::Result<()> {
        let updated_at = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        self.values.insert(
            conversation,
            Binding {
                account,
                reason: reason.to_owned(),
                updated_at,
            },
        );
        self.persist()
    }

    fn persist(&self) -> io::Result<()> {
        let parent = self.path.parent().unwrap_or_else(|| Path::new("."));
        fs::create_dir_all(parent)?;
        let data = serde_json::to_vec_pretty(&self.values).map_err(io::Error::other)?;
        let temporary = self.path.with_extension("tmp");
        fs::write(&temporary, data)?;
        fs::rename(temporary, &self.path)
    }
}

#[cfg(test)]
mod tests {
    use super::BindingStore;
    use std::fs;

    #[test]
    fn persists_binding_across_reload() {
        let directory =
            std::env::temp_dir().join(format!("cm-binding-test-{}", std::process::id()));
        let path = directory.join("bindings.json");
        let mut store = BindingStore::load(path.clone());
        store
            .bind("chat-1".into(), "account-a".into(), "selected")
            .unwrap();
        let reloaded = BindingStore::load(path);
        assert_eq!(reloaded.account("chat-1"), Some("account-a"));
        let _ = fs::remove_dir_all(directory);
    }
}
