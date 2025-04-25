#!/usr/bin/env python3

import hashlib
import os
import urllib.request
import tarfile
import shutil
import logging
from argparse import ArgumentParser

format_string = (
    "%(levelname)-5.5s [%(name)s:%(lineno)s][%(threadName)s] %(message)s"
)
log_level = os.getenv("LOGLEVEL", "INFO")
logging.basicConfig(format=format_string, level=log_level)
logger = logging.getLogger(__name__)


class GeoIPUpdater:
    def __init__(self, cdn, database):
        self.maxmind_url = f"{cdn}/geoip2-database/{database}.tar.gz"
        self.maxmind_sha256_url = (
            f"{cdn}/geoip2-database/{database}.tar.gz.sha256"
        )
        self.local_filename = f"{database}.mmdb"

    def match_sha256(self, _file):
        """
        Purpose of the method to make sure downloaded file is accurate.
        Args:
            _file (file): downloaded file
        Returns:
            bool: Returns True if downloaded file hash and hash returned by url
            are same otherwise returns False
        """
        sha256 = urllib.request.urlopen(self.maxmind_sha256_url).read()
        hash_object = hashlib.sha256()
        file_content = open(_file, "rb").read()
        hash_object.update(file_content)

        return hash_object.hexdigest() in sha256.decode()

    @staticmethod
    def write(outfile, existing_file):
        """
        Copy the newly downloaded file to existing file
        Args:
            outfile (File): downloaded file from url
            existing_file (File): existing file in directory
        Returns:
            None
        """
        with open(existing_file, "wb") as exfile:
            shutil.copyfileobj(outfile, exfile)

    def download_file(self, output_directory):
        """
        Downloads the file and check sha256 hash to make sure file is downloaded correctly.
        If doesn't match then it would abort operation otherwise continue copying the file
        Args:
            output_directory: directory where all files will be saved to
        Returns:
            None
        Raises:
            ValueError: if sha256 hash of downloaded file doesn't match
        """
        logger.info("Downloading maxmind %s database", self.local_filename)
        existing_file = os.path.join(output_directory, self.local_filename)
        downloaded_file, headers = urllib.request.urlretrieve(self.maxmind_url)

        if self.match_sha256(downloaded_file):

            with tarfile.open(downloaded_file, "r:gz") as outfile:
                for member in outfile.getmembers():
                    if self.local_filename in member.name:
                        _file = outfile.extractfile(member)
                        self.write(_file, existing_file)
            os.remove(downloaded_file)
        else:
            logger.error(
                "Downloaded file hash did't matched. Aborting the operation"
            )
            try:
                os.remove(downloaded_file)
            finally:
                raise ValueError(
                    f"sha256 of {downloaded_file} doesn't match the signature."
                )


def parse_arguments():
    parser = ArgumentParser()
    parser.add_argument(
        "-c",
        "--cdn",
        type=str,
        help="CDN endpoint",
    )
    parser.add_argument(
        "-d",
        "--database",
        type=str,
        help="MaxMind database type",
        choices=["GeoIP2-City", "GeoIP2-Country"],
    )
    parser.add_argument("-o", "--output", type=str, help="Output folder")
    return parser.parse_args()


def main():
    args = parse_arguments()
    updater = GeoIPUpdater(args.cdn, args.database)
    updater.download_file(args.output)


if __name__ == "__main__":
    main()
