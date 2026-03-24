import { Folders, Sparkles } from "lucide-react";

export function CollectionsPage() {
  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">Collections</p>
          <h3>这里将承接逻辑分类，而不是底层物理存储结构。</h3>
          <p>
            智能文件夹、项目集合、星级与颜色标签都会挂在这一层，和存储节点目录树分离。
          </p>
        </div>

        <div className="hero-metrics">
          <article className="metric-card tone-neutral">
            <p>状态</p>
            <strong>规划中</strong>
          </article>
          <article className="metric-card tone-warning">
            <p>下一步</p>
            <strong>收藏夹 / 智能集</strong>
          </article>
        </div>
      </article>

      <article className="detail-card empty-state">
        <Folders size={28} />
        <div>
          <h4>Collections 入口已预留</h4>
          <p>
            这一页现在先作为占位，后续会放入逻辑分类、标签体系和基于规则的智能集合。
          </p>
        </div>
        <span className="status-pill subtle">
          <Sparkles size={14} />
          不再把物理端点当成主要浏览入口
        </span>
      </article>
    </section>
  );
}
