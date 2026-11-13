import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("package-release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ContentPackagingTest(unittest.TestCase):
    def write(self, root, relative, value):
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value))
        return path

    def project(self, root, identity):
        self.write(root, "adventure.json", {"version": 1, "id": identity, "scenarios": {"en": "world/story.json"}, "catalogue_source": "library"})
        self.write(root, "world/story.json", {"version": 1, "locations": {"room": {}}, "id": identity})
        self.write(root, "library/category/entries.json", {"version": 1, "entries": [], "assets": [{"id": "background", "path": "picture.png"}]})
        (root / "library/category/picture.png").write_bytes(b"image fixture")
        (root / "library/private.sqlite").write_bytes(b"must not ship")
        (root / "world/unused.json").write_text("{}")

    def test_any_project_can_supply_content(self):
        with tempfile.TemporaryDirectory() as temp:
            for identity in ("forest", "station"):
                root, output = Path(temp)/identity, Path(temp)/(identity+"-package")
                self.project(root, identity)
                release.playable_content(root, output, "adventure.json")
                config = json.loads((output/"game.json").read_text())
                self.assertEqual(config["id"], identity)
                self.assertEqual({str(p.relative_to(output)) for p in output.rglob("*") if p.is_file()},
                    {"game.json", "world/story.json", "library/category/entries.json", "library/category/picture.png"})
                # A staged tree remains self-contained when it becomes an app.
                release.playable_content(output, Path(temp)/(identity+"-app"))

    def test_outside_images_and_private_files_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)/"game"
            self.project(root,"test")
            for path in ("../../private.gguf", "../private.sqlite"):
                self.write(root,"library/category/entries.json",{"version":1,"assets":[{"id":"x","path":path}]})
                (root/"private.gguf").write_bytes(b"model")
                with self.assertRaises(ValueError):
                    release.playable_content(root,Path(temp)/"output","adventure.json")

    def test_alias_image_keeps_the_manifest_path(self):
        with tempfile.TemporaryDirectory() as temp:
            root,output=Path(temp)/"game",Path(temp)/"out"
            self.project(root,"test")
            folder=root/"library/category"
            (folder/"alias.png").symlink_to("picture.png")
            self.write(root,"library/category/entries.json",{"version":1,"assets":[{"id":"x","path":"alias.png"}]})
            release.playable_content(root,output,"adventure.json")
            self.assertEqual((output/"library/category/alias.png").read_bytes(),b"image fixture")

    def test_duplicate_configuration_fields_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp)
            (root/"game.json").write_text('{"version":1,"version":1}')
            with self.assertRaises(ValueError):
                release.playable_content(root,root/"out")


if __name__ == "__main__":
    unittest.main()
