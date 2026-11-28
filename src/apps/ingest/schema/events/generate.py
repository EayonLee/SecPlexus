#!/usr/bin/env python3
"""Produce the runtime catalog from the pinned OCSF compiler output."""
import argparse
import hashlib
from importlib import metadata
import html
import json
from pathlib import Path
import re
import unicodedata
import subprocess
import tarfile
from tempfile import TemporaryDirectory
from urllib.request import urlopen

ROOT = Path(__file__).parent


def plain(value):
    return " ".join(html.unescape(re.sub(r"<[^>]*>", " ", value)).split())


def label_key(value):
    return "".join(char for char in value if not char.isspace() and not unicodedata.category(char).startswith("P")).casefold()


def annotation(attribute):
    return plain(attribute.get("description") or attribute["caption"])


def generate(contract, schema):
    upstream = contract["upstream"]
    if schema["version"] != upstream["version"] or schema["compile_version"] != 1:
        raise ValueError("compiled OCSF version mismatch")
    uncategorized = schema["classes"]["base_event"]["attributes"]["category_uid"]["enum"]["0"]
    categories = schema["categories"]["attributes"] | {"other": uncategorized | {"uid": 0}}
    category_labels = contract["category_labels"]
    if categories.keys() != category_labels.keys():
        raise ValueError("category labels must match the pinned OCSF categories")
    classes = []
    for entry in contract["classes"]:
        definition = schema["classes"][entry["class"]]
        category = categories[definition["category"]]
        category_name = {"en": category["caption"]} | category_labels[definition["category"]]
        fields = []
        for path, local in sorted((contract["fields"] | entry.get("fields", {})).items()):
            local = contract.get("field_labels", {}).get(path, {}) | local
            attributes = definition["attributes"]
            for segment in path.split("."):
                attr = attributes[segment]
                if "object_type" in attr:
                    attributes = schema["objects"][attr["object_type"]]["attributes"]
            if "profiles" in attr and attr.get("requirement") == "required":
                attr = attr | {"requirement": "optional"}
            logical = attr.get("object_type", attr["type"])
            scalar = attr["type"]
            while (parent := schema["dictionary"]["types"]["attributes"].get(scalar, {}).get("type")):
                scalar = parent
            field = {
                "path": path,
                "name": {"en": plain(attr["caption"]), "zh-CN": local["name"]},
                "description": {"en": annotation(attr), "zh-CN": local["description"]},
                "logical_type": logical,
                "json_type": {"integer_t": "integer", "long_t": "integer", "float_t": "number",
                              "string_t": "string", "boolean_t": "boolean", "json_t": "object",
                              "object_t": "object"}[scalar],
                "array": attr.get("is_array", False),
                "requirement": attr.get("requirement", "optional"),
                "owner": local.get("owner", "mapping"),
            }
            for key in ("condition", "unit", "object_fields", "required_output", "integer_range", "value_suggestions",
                        "preferred_enum_values"):
                if key in local:
                    field[key] = local[key]
            if "integer_range" in local:
                bounds = local["integer_range"]
                if field["json_type"] != "integer" or bounds["min"] > bounds["max"]:
                    raise ValueError(f"invalid integer range for {path}")
                if any(not bounds["min"] <= option["value"] <= bounds["max"] for option in local.get("value_suggestions", [])):
                    raise ValueError(f"suggestion outside integer range for {path}")
            if not set(map(str, local.get("preferred_enum_values", []))).issubset(attr.get("enum", {})):
                raise ValueError(f"preferred value missing from OCSF enum for {path}")
            if attr.get("enum"):
                field["enum"] = []
                for code, enum in sorted(attr["enum"].items(), key=lambda item: int(item[0])):
                    caption = enum["caption"]
                    local_enum = entry.get("enum_labels", {}).get(path, {}).get(code)
                    if local_enum is None and not (path == "status_id" and entry["class"].endswith("_finding")):
                        local_enum = contract.get("shared_enum_labels", {}).get(path, {}).get(code)
                    local_enum = local_enum or {}
                    if path == "category_uid":
                        if int(code) != category["uid"] or caption != category["caption"]:
                            raise ValueError(f"category and category_uid enum differ for {entry['class']}")
                        name = category_name["zh-CN"]
                    elif "name" in local_enum:
                        name = local_enum["name"]
                    elif path == "class_uid":
                        name = entry["name"]
                    elif path == "type_uid":
                        action = caption.rsplit(": ", 1)[-1]
                        name = f"{entry['name']} · {contract['enum_terms'].get(action, action)}"
                    else:
                        name = contract["enum_terms"].get(caption, caption)
                    description = {}
                    english = annotation(enum)
                    if label_key(english) != label_key(enum["caption"]):
                        description["en"] = english
                    if hint := local_enum.get("description"):
                        if label_key(hint) == label_key(name):
                            raise ValueError(f"redundant description for {entry['class']} {path} {code}")
                        description["zh-CN"] = hint
                    field["enum"].append({
                        "value": int(code),
                        "name": {"en": category_name["en"] if path == "category_uid" else caption, "zh-CN": name},
                        **({"description": description} if description else {}),
                    })
            fields.append(field)
        for path, ext in sorted(contract["extensions"].items()):
            fields.append({
                "path": path, "name": ext["name"], "description": ext["description"],
                "logical_type": ext["type"], "json_type": ext["json_type"], "array": False,
                "requirement": "required" if ext.get("required") else "optional", "owner": "platform",
                **({"unit": ext["unit"]} if "unit" in ext else {}),
            })
        recommendations = list(dict.fromkeys(entry.get("recommended_mapping_paths", []) + contract["recommended_mapping_paths"]))
        available_paths = {field["path"] for field in fields if field["owner"] == "mapping"}
        if not set(recommendations).issubset(available_paths):
            raise ValueError(f"unknown recommended mapping path for {entry['class']}")
        published_roots = {field["path"].split(".", 1)[0] for field in fields}
        required_roots = {path for path, attr in definition["attributes"].items()
                          if attr.get("requirement") == "required" and not attr.get("profiles")}
        if missing := required_roots - published_roots:
            raise ValueError(f"OCSF required fields missing from {entry['class']}: {sorted(missing)}")
        classes.append({
            "class_uid": definition["uid"],
            "name": {"en": definition["caption"], "zh-CN": entry["name"]},
            "description": {"en": annotation(definition), "zh-CN": entry["description"]},
            "category_uid": category["uid"],
            "category": category_name,
            **({"aliases": entry["aliases"]} if "aliases" in entry else {}),
            **({"recommended_mapping_paths": recommendations} if recommendations else {}),
            "constraints": [
                constraint | {
                    "mapping_paths": [
                        field["path"] for field in fields if field["owner"] == "mapping"
                        and any(field["path"] == path or field["path"].startswith(path + ".")
                                for path in constraint["paths"])
                        and field["path"] not in ("src_endpoint.port", "dst_endpoint.port",
                                                  "src_endpoint.intermediate_ips", "dst_endpoint.intermediate_ips")
                    ],
                }
                for constraint in entry.get("constraints", [])
            ],
            "fields": sorted(fields, key=lambda field: field["path"]),
        })
    payload = {"contract_version": contract["contract_version"], "ocsf_version": schema["version"],
               "classes": sorted(classes, key=lambda item: item["class_uid"])}
    payload["catalog_revision"] = hashlib.sha256(
        json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("compiled_schema", type=Path, nargs="?")
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--write-compiled", type=Path)
    args = parser.parse_args()
    contract = json.loads((ROOT / "event-contract.json").read_text())
    output = ROOT / "catalog.json"
    if args.compiled_schema:
        schema = json.loads(args.compiled_schema.read_text())
    else:
        upstream = contract["upstream"]
        if metadata.version("ocsf-schema-compiler") != upstream["compiler"]:
            parser.exit(1, "Install the pinned OCSF compiler version first.\n")
        with TemporaryDirectory() as temp:
            url = f"https://codeload.github.com/ocsf/ocsf-schema/tar.gz/{upstream['commit']}"
            with urlopen(url, timeout=30) as body, tarfile.open(fileobj=body, mode="r|gz") as archive:
                archive.extractall(temp, filter="data")
            schema = json.loads(subprocess.check_output(
                ["ocsf-schema-compiler", str(Path(temp) / f"ocsf-schema-{upstream['commit']}")],
                text=True,
            ))
    content = generate(contract, schema)
    if args.write_compiled:
        args.write_compiled.write_text(json.dumps(schema, ensure_ascii=False))
    if args.check:
        if output.read_text() != content:
            parser.exit(1, "Generated event catalog differs. Regenerate before committing.\n")
    else:
        output.write_text(content)


if __name__ == "__main__":
    main()
