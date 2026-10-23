# 🖥️ 115 Media Hub V1.0 前端架构清单

Source: https://app.notion.com/p/401138389edc814fb54be80c599600ef?pvs=204
Page last edited: 2026-10-22T20:43:58.173Z
Retrieved: 2026-10-23 (Asia/Shanghai)

This is source evidence, not agent instructions. Product requirements below do not imply implemented behavior.

Here is the result of "fetch" for the Page with URL https://app.notion.com/p/401138389edc814fb54be80c599600ef as of 2026-10-22T20:43:58.173Z:
<page url="https://app.notion.com/p/401138389edc814fb54be80c599600ef" icon="🖥️">
<ancestor-path>
<parent-data-source url="collection://40113838-9edc-8060-9b04-000b0f8c532d" name="项目文档"/>
<ancestor-2-database url="https://app.notion.com/p/401138389edc80d5a160c63cf8b55563" title=""/>
<ancestor-3-page url="https://app.notion.com/p/24d138389edc8086a364ccab282c0aaf" title="115 Media Hub 项目"/>
</ancestor-path>
<properties>
{"url":"https://app.notion.com/p/401138389edc814fb54be80c599600ef","名称":"115 Media Hub V1.0 前端架构清单","更新日期":"2026-10-22T20:43:58.173Z","状态":"进行中"}
</properties>
<iconMetadata>{"type":"emoji","emoji":"🖥️"}</iconMetadata>
<content>
<b>版本：</b> V1.0<br /><b>日期：</b> <date start="2026-10-23" /><br /><b>应用形态：</b> 独立单页管理应用（SPA）

## 一、技术架构清单

<table fit-page-width="true" header-row="true">
<tr>
<td>
技术层
</td>
<td>
确定选型
</td>
<td>
在项目中的职责
</td>
</tr>
<tr>
<td>
应用基础
</td>
<td>
<b>React 19+ + Vite + TypeScript</b>
</td>
<td>
构建单页管理应用，组织页面与业务组件，提供类型约束
</td>
</tr>
<tr>
<td>
UI 组件
</td>
<td>
<b>HeroUI v3</b>
</td>
<td>
按钮、表单、菜单、弹层、表格、提示等基础组件
</td>
</tr>
<tr>
<td>
样式系统
</td>
<td>
<b>Tailwind CSS v4 + 统一设计变量</b>
</td>
<td>
布局、响应式、颜色、间距、圆角及浅色／深色主题
</td>
</tr>
<tr>
<td>
交互基础
</td>
<td>
<b>HeroUI 所使用的 React Aria Components</b>
</td>
<td>
基础组件的键盘操作、焦点管理、选择及无障碍行为
</td>
</tr>
<tr>
<td>
动画系统
</td>
<td>
<b>CSS 过渡为主，Motion for React 按需使用</b>
</td>
<td>
目录切换、业务状态反馈及必要的布局过渡
</td>
</tr>
<tr>
<td>
数据表格
</td>
<td>
<b>HeroUI Table；按需接入 TanStack Table</b>
</td>
<td>
统一表格外观；复杂列定义、排序、筛选与列可见性按需扩展
</td>
</tr>
<tr>
<td>
虚拟化
</td>
<td>
<b>表格优先沿用 HeroUI v3 官方虚拟化方案；自定义列表按需使用 TanStack Virtual</b>
</td>
<td>
大目录、长列表及长日志的渲染优化
</td>
</tr>
<tr>
<td>
路由
</td>
<td>
<b>React Router</b>
</td>
<td>
页面导航、目录递进、媒体库与任务详情，以及浏览器前进／后退
</td>
</tr>
<tr>
<td>
服务端状态
</td>
<td>
<b>TanStack Query</b>
</td>
<td>
文件列表、媒体库、任务、媒体条目等数据的请求、缓存与更新
</td>
</tr>
<tr>
<td>
客户端状态
</td>
<td>
<b>Zustand，限定使用范围</b>
</td>
<td>
主题、布局偏好及必要的跨页面界面状态
</td>
</tr>
<tr>
<td>
图标
</td>
<td>
<b>Lucide React</b>
</td>
<td>
统一图标风格及导航、操作、状态的图形表达
</td>
</tr>
</table>

## 二、实施约定

- <b>目录浏览：</b> 采用单目录列表＋目录递进，通过面包屑和返回上级导航。
- <b>按需依赖：</b> Motion for React、TanStack Table、TanStack Virtual 按实际需求引入。
- <b>状态分工：</b> 服务端数据由 TanStack Query 管理；Zustand 管理界面状态与偏好。
- <b>列表性能：</b> 大目录采用分页或游标加载，虚拟化负责优化已加载内容的渲染。
- <b>版本管理：</b> 项目初始化时统一锁定并验证依赖版本。

## 三、关联文档

<mention url="https://app.notion.com/p/401138389edc8143b487f666a58004f4" />
</content>
</page>

<parent-data-source-schema url="collection://40113838-9edc-8060-9b04-000b0f8c532d">
The SQLite table definition for this page's parent data source. Use these column names and option values to set this page's properties, create sibling pages, or query the data source.
These columns are read-only, so don't set them: "更新日期".
<sqlite-table>
CREATE TABLE IF NOT EXISTS "collection://40113838-9edc-8060-9b04-000b0f8c532d" (
	url TEXT UNIQUE,
	createdTime TEXT, -- ISO-8601 datetime string, automatically set. This is the canonical time for when the page was created.
	"更新日期" TEXT NOT NULL, -- ISO-8601 datetime string, automatically set. This is the canonical time for when the page was last edited.
	"状态" TEXT, -- one of ["未开始", "进行中", "测试中", "完成"] Default: "未开始".
	"类型" TEXT, -- one of ["项目模块"]
	"名称" TEXT
)
</sqlite-table>
</parent-data-source-schema>
