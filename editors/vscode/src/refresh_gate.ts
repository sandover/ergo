export class RefreshGate {
  private revision = 0;
  private disposed = false;

  async run<T>(load: () => Promise<T>, apply: (value: T) => void): Promise<void> {
    const revision = ++this.revision;
    try {
      const value = await load();
      if (!this.disposed && revision === this.revision) {
        apply(value);
      }
    } catch (error) {
      if (!this.disposed && revision === this.revision) {
        throw error;
      }
    }
  }

  dispose(): void {
    this.disposed = true;
    this.revision++;
  }
}
