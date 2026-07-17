from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]
SKILLS = ROOT / "skills"
EXPECTED_SKILLS = {
    "analyze-codex-session",
    "analyze-data-quality",
    "build-dashboard",
    "build-report",
    "create-data-context",
    "design-kpis",
    "validate-data",
    "visualize-data",
}


class PackagedSkillTests(unittest.TestCase):
    def test_requested_analytics_skills_are_packaged(self) -> None:
        packaged = {path.name for path in SKILLS.iterdir() if path.is_dir()}
        self.assertEqual(packaged, EXPECTED_SKILLS)

    def test_skill_metadata_is_complete_and_source_aware(self) -> None:
        for name in sorted(EXPECTED_SKILLS):
            with self.subTest(skill=name):
                skill = (SKILLS / name / "SKILL.md").read_text(encoding="utf-8")
                metadata = (SKILLS / name / "agents" / "openai.yaml").read_text(encoding="utf-8")
                self.assertIn(f"name: {name}", skill)
                self.assertNotIn("[TODO", skill)
                self.assertIn("/codex", skill)
                self.assertIn("provenance", skill.lower())
                self.assertIn("display_name:", metadata)
                self.assertIn("short_description:", metadata)
                self.assertIn(f"${name}", metadata)


if __name__ == "__main__":
    unittest.main()
