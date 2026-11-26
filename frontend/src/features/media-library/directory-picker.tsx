import { Alert, Breadcrumbs, Button, Modal } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Folder, X } from "lucide-react";
import { useState } from "react";

import type { DirectoryListing } from "@/features/cloud-drive/api";
import { getDirectory } from "@/features/cloud-drive/api";

export function DirectoryPicker({
  title = "选择 115 网盘目录",
  onSelect,
  onClose,
}: {
  title?: string;
  onSelect: (id: string, path: string) => void;
  onClose: () => void;
}) {
  const [navigationPath, setNavigationPath] = useState<
    DirectoryListing["path"]
  >([{ id: "0", name: "115 网盘" }]);
  const directory = navigationPath.at(-1)?.id ?? "0";
  const [cursor, setCursor] = useState<string | null>(null);
  const query = useQuery({
    queryKey: ["library-directory", directory, cursor],
    queryFn: ({ signal }) => {
      const params = new URLSearchParams({
        directory_id: directory,
        limit: "200",
        sort: "name",
        order: "asc",
        type: "all",
      });
      if (cursor) params.set("cursor", cursor);
      return getDirectory(params, signal);
    },
    retry: false,
  });
  // The destination path stays visible while its directory request is pending.
  const breadcrumbPath = query.data?.path ?? navigationPath;
  const path = query.data?.path.map((part) => part.name).join("/") ?? "";
  return (
    <Modal.Backdrop
      isOpen
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <Modal.Container>
        <Modal.Dialog aria-labelledby="directory-title">
          <header className="flex items-center justify-between gap-3">
            <h2 id="directory-title">{title}</h2>
            <Button
              variant="ghost"
              type="button"
              onPress={onClose}
              aria-label="关闭"
            >
              <X />
            </Button>
          </header>
          <p>选择要绑定的目录，之后可在编辑配置中更改。</p>
          <nav className="overflow-x-auto" aria-label="目录路径">
            <Breadcrumbs separator="/">
              {breadcrumbPath.map((part, index) => (
                <Breadcrumbs.Item
                  key={part.id}
                  {...(index === breadcrumbPath.length - 1
                    ? {}
                    : {
                        onPress: () => {
                          setNavigationPath(breadcrumbPath.slice(0, index + 1));
                          setCursor(null);
                        },
                      })}
                >
                  {part.name}
                </Breadcrumbs.Item>
              ))}
            </Breadcrumbs>
          </nav>
          <div className="my-3 h-72 overflow-auto" aria-busy={query.isPending}>
            {query.isPending ? (
              <p role="status">正在读取目录…</p>
            ) : query.isError ? (
              <Alert status="danger" role="alert">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Description>
                    无法读取目录，请检查 115 授权或网络后重试。
                  </Alert.Description>
                </Alert.Content>
                <Button
                  variant="ghost"
                  type="button"
                  onPress={() => void query.refetch()}
                >
                  重试
                </Button>
              </Alert>
            ) : (
              <div className="flex flex-col items-stretch">
                {query.data.items
                  .filter((item) => item.is_directory)
                  .map((item) => (
                    <Button
                      variant="ghost"
                      type="button"
                      key={item.id}
                      onPress={() => {
                        setNavigationPath([
                          ...breadcrumbPath,
                          { id: item.id, name: item.name },
                        ]);
                        setCursor(null);
                      }}
                    >
                      <Folder /> {item.name}
                      <ChevronRight />
                    </Button>
                  ))}
                {query.data.items.every((item) => !item.is_directory) && (
                  <p>此页没有文件夹</p>
                )}
              </div>
            )}
            {query.data?.next_cursor && (
              <Button
                variant="ghost"
                type="button"
                onPress={() => setCursor(query.data.next_cursor)}
              >
                下一页
              </Button>
            )}
            {cursor && (
              <Button
                variant="ghost"
                type="button"
                onPress={() => setCursor(null)}
              >
                返回第一页
              </Button>
            )}
          </div>
          <footer className="mt-4 flex justify-end gap-2">
            <Button variant="ghost" type="button" onPress={onClose}>
              取消
            </Button>
            <Button
              variant="primary"
              type="button"
              isDisabled={!query.data || query.isFetching || query.isError}
              onPress={() => onSelect(directory, path)}
            >
              选择当前目录
            </Button>
          </footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
